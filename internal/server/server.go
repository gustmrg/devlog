package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	githubcollector "devlog/internal/collector/github"
	"devlog/internal/config"
	"devlog/internal/correlation"
	"devlog/internal/database"
	"devlog/internal/domain"
	discordnotify "devlog/internal/notify/discord"
	summarygen "devlog/internal/summary"
	"devlog/internal/syncapi"
	webassets "devlog/web"
	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
)

type Server struct {
	Config                 config.Config
	DB                     *database.DB
	PairingCode, PublicURL string
	adminSalt, adminHash   []byte
	templates              *template.Template
	sessions               map[string]time.Time
	mu                     sync.Mutex
	workMu                 sync.Mutex
	jobMu                  sync.Mutex
	runningJobs            map[string]bool
	notifier               *discordnotify.Notifier
	pairing                *syncapi.Pairing
	fallback               summarygen.Deterministic
}

func New(cfg config.Config, dataDir, pairingCode, adminPassword, publicURL string) (*Server, error) {
	db, err := database.Open(filepath.Join(dataDir, "devlog.db"))
	if err != nil {
		return nil, err
	}
	location, _ := time.LoadLocation(cfg.Schedules.Timezone)
	t, err := template.New("web").Funcs(template.FuncMap{"local": func(value time.Time) time.Time { return value.In(location) }}).ParseFS(webassets.Files, "templates/*.html")
	if err != nil {
		db.Close()
		return nil, err
	}
	if pairingCode == "" {
		pairingCode = randomCode()
	}
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	var adminHash []byte
	if adminPassword != "" {
		adminHash = argon2.IDKey([]byte(adminPassword), salt, 1, 64*1024, 4, 32)
	}
	s := &Server{Config: cfg, DB: db, PairingCode: pairingCode, adminSalt: salt, adminHash: adminHash, PublicURL: strings.TrimRight(publicURL, "/"), templates: t, sessions: map[string]time.Time{}, runningJobs: map[string]bool{}}
	s.pairing = syncapi.NewPairing(pairingCode, 10*time.Minute)
	ctx := context.Background()
	for _, p := range cfg.Projects {
		_ = db.UpsertProject(ctx, domain.Project{ID: p.ID, Name: p.Name, CanonicalRemote: p.Remote, Enabled: p.Enabled, CreatedAt: time.Now().UTC()})
	}
	discordToken := ""
	if cfg.Discord.Enabled {
		discordToken = os.Getenv(cfg.Discord.TokenEnvVar)
	}
	s.notifier = &discordnotify.Notifier{Token: discordToken, ChannelID: cfg.Discord.ChannelID, AllowedUserID: cfg.Discord.UserID, PublicURL: s.PublicURL,
		OnConfirm:    func(ctx context.Context, id string) error { return s.DB.ConfirmSummaryAndActivities(ctx, id) },
		OnRegenerate: func(ctx context.Context, date string) error { return s.GenerateSummary(ctx, date) },
		OnAdd: func(ctx context.Context, date, description string) error {
			return s.AddActivity(ctx, date, description, "")
		},
		OnEditSummary: func(ctx context.Context, id, content string) error {
			return s.DB.UpdateSummaryContent(ctx, id, content)
		},
	}
	return s, nil
}

func (s *Server) Close() error { _ = s.notifier.Close(); return s.DB.Close() }
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	syncHandler := syncapi.Handler{DB: s.DB, Pairing: s.pairing, OnEvents: func(events []domain.Event) { go s.handleArrivingEvents(events) }}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok\n")) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.DB.PingContext(r.Context()); err != nil {
			http.Error(w, "not ready", 503)
			return
		}
		_, _ = w.Write([]byte("ready\n"))
	})
	mux.HandleFunc("POST /api/v1/devices/pair", syncHandler.Pair)
	mux.HandleFunc("POST /api/v1/sync/events", syncHandler.Push)
	mux.HandleFunc("GET /api/v1/timeline", syncHandler.Timeline)
	mux.HandleFunc("GET /api/v1/sync/changes", syncHandler.Changes)
	mux.Handle("GET /assets/", http.FileServerFS(webassets.Files))
	mux.HandleFunc("GET /login", s.loginPage)
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("POST /logout", s.logout)
	mux.Handle("GET /", s.requireSession(http.HandlerFunc(s.timeline)))
	mux.Handle("GET /days/{date}", s.requireSession(http.HandlerFunc(s.timeline)))
	mux.Handle("GET /devices", s.requireSession(http.HandlerFunc(s.devices)))
	mux.Handle("POST /devices/pairing-code", s.requireSession(http.HandlerFunc(s.rotatePairingCode)))
	mux.Handle("POST /devices/{id}/revoke", s.requireSession(http.HandlerFunc(s.revokeDevice)))
	mux.Handle("GET /projects", s.requireSession(http.HandlerFunc(s.projects)))
	mux.Handle("GET /jobs", s.requireSession(http.HandlerFunc(s.jobs)))
	mux.Handle("GET /settings", s.requireSession(http.HandlerFunc(s.settings)))
	mux.Handle("POST /projects", s.requireSession(http.HandlerFunc(s.createProject)))
	mux.Handle("POST /projects/{id}/enabled", s.requireSession(http.HandlerFunc(s.setProjectEnabled)))
	mux.Handle("POST /activities/{id}/status", s.requireSession(http.HandlerFunc(s.activityStatus)))
	mux.Handle("POST /activities/{id}/edit", s.requireSession(http.HandlerFunc(s.editActivity)))
	mux.Handle("POST /activities", s.requireSession(http.HandlerFunc(s.createActivity)))
	mux.Handle("POST /summaries/{date}/regenerate", s.requireSession(http.HandlerFunc(s.regenerate)))
	mux.Handle("POST /summaries/{id}/confirm", s.requireSession(http.HandlerFunc(s.confirmSummary)))
	return securityHeaders(mux)
}

func (s *Server) Run(ctx context.Context, address string) error {
	if len(s.adminHash) == 0 {
		return errors.New("DEVLOG_ADMIN_PASSWORD is required")
	}
	if err := s.notifier.Start(); err != nil {
		return fmt.Errorf("start Discord: %w", err)
	}
	go s.schedule(ctx)
	srv := &http.Server{Addr: address, Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Printf("devlog serve listening on %s; pairing code: %s", address, s.PairingCode)
	err := srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Correlate(ctx context.Context, date string) error {
	s.workMu.Lock()
	defer s.workMu.Unlock()
	return s.correlate(ctx, date)
}
func (s *Server) correlate(ctx context.Context, date string) error {
	loc, _ := time.LoadLocation(s.Config.Schedules.Timezone)
	events, err := s.DB.EventsForDay(ctx, date, loc)
	if err != nil {
		return err
	}
	activities := correlation.Correlator{IdleGap: 45 * time.Minute}.Correlate(events)
	return s.DB.ReplaceActivitiesForDay(ctx, date, activities)
}
func (s *Server) GenerateSummary(ctx context.Context, date string) error {
	s.workMu.Lock()
	defer s.workMu.Unlock()
	if err := s.correlate(ctx, date); err != nil {
		return err
	}
	activities, err := s.DB.ActivitiesForDay(ctx, date)
	if err != nil {
		return err
	}
	content, err := s.fallback.Generate(ctx, date, activities)
	if len(activities) == 0 {
		content = "Poucos sinais foram encontrados hoje. Foi um dia de reuniões, estudo ou outra atividade sem rastro digital?"
	}
	if len(activities) > 0 && s.Config.LLM.Enabled && os.Getenv(s.Config.LLM.APIKeyEnvVar) != "" {
		g := summarygen.OpenAICompatible{BaseURL: s.Config.LLM.BaseURL, APIKey: os.Getenv(s.Config.LLM.APIKeyEnvVar), Model: s.Config.LLM.Model, Language: s.Config.Defaults.Language, Style: s.Config.Defaults.Style}
		if generated, e := g.Generate(ctx, date, activities); e == nil {
			content = generated
		} else {
			log.Printf("LLM fallback: %v", e)
		}
	}
	if err != nil {
		return err
	}
	rev, err := s.DB.NextSummaryRevision(ctx, date)
	if err != nil {
		return err
	}
	summary := domain.Summary{ID: uuid.NewString(), Date: date, Revision: rev, Content: content, Status: "draft", CreatedAt: time.Now().UTC()}
	if err := s.DB.SaveSummary(ctx, summary); err != nil {
		return err
	}
	return s.notifier.SendSummary(ctx, summary)
}
func (s *Server) AddActivity(ctx context.Context, date, description, projectID string) error {
	if strings.TrimSpace(description) == "" {
		return errors.New("description is required")
	}
	loc, _ := time.LoadLocation(s.Config.Schedules.Timezone)
	day, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return err
	}
	now := time.Now().In(loc)
	at := time.Date(day.Year(), day.Month(), day.Day(), now.Hour(), now.Minute(), 0, 0, loc)
	return s.DB.CreateActivity(ctx, date, domain.Activity{ID: uuid.NewString(), ProjectID: projectID, Description: strings.TrimSpace(description), StartedAt: at, EndedAt: at, Status: domain.ActivityConfirmed, Confidence: domain.ConfidenceHigh, UpdatedAt: time.Now().UTC()})
}
func (s *Server) CollectGitHub(ctx context.Context) error {
	if !s.Config.GitHub.Enabled {
		return nil
	}
	token := os.Getenv(s.Config.GitHub.TokenEnvVar)
	projects, err := s.DB.Projects(ctx)
	if err != nil {
		return err
	}
	for _, p := range projects {
		if !p.Enabled {
			continue
		}
		owner, repo, ok := githubcollector.ParseRemote(p.CanonicalRemote)
		if !ok {
			continue
		}
		key := owner + "/" + repo
		cursor, _ := s.DB.Cursor(ctx, "github", key)
		c := githubcollector.Collector{Token: token, Owner: owner, Repo: repo, ProjectID: p.ID}
		events, next, err := c.Collect(ctx, cursor)
		if err != nil {
			return err
		}
		n, err := s.DB.InsertEvents(ctx, events)
		if err != nil {
			return err
		}
		if n > 0 {
			go s.handleArrivingEvents(events)
		}
		if err := s.DB.SetCursor(ctx, "github", key, next); err != nil {
			return err
		}
	}
	return nil
}
func (s *Server) handleArrivingEvents(events []domain.Event) {
	dates := map[string]bool{}
	for _, event := range events {
		dates[s.local(event.OccurredAt).Format("2006-01-02")] = true
	}
	for date := range dates {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		if err := s.Correlate(ctx, date); err != nil {
			log.Printf("late correlation: %v", err)
			cancel()
			continue
		}
		if _, err := s.DB.LatestSummary(ctx, date); err == nil {
			if err := s.GenerateSummary(ctx, date); err != nil {
				log.Printf("late summary revision: %v", err)
			}
		}
		cancel()
	}
}

func (s *Server) schedule(ctx context.Context) {
	interval, _ := time.ParseDuration(s.Config.Schedules.CorrelateInterval)
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	githubInterval, _ := time.ParseDuration(s.Config.GitHub.Interval)
	if githubInterval <= 0 {
		githubInterval = 15 * time.Minute
	}
	correlateTicker := time.NewTicker(interval)
	githubTicker := time.NewTicker(githubInterval)
	minute := time.NewTicker(time.Minute)
	defer correlateTicker.Stop()
	defer githubTicker.Stop()
	defer minute.Stop()
	lastSummary := ""
	lastFinalize := ""
	lastRetention := ""
	for {
		select {
		case <-ctx.Done():
			return
		case <-githubTicker.C:
			s.runJob(ctx, "github.collect", func() error { return s.CollectGitHub(ctx) })
		case <-correlateTicker.C:
			s.runJob(ctx, "correlate", func() error { return s.Correlate(ctx, s.today()) })
		case now := <-minute.C:
			local := s.local(now)
			date := local.Format("2006-01-02")
			if local.Format("15:04") == s.Config.Schedules.FinalizeAt && lastFinalize != date {
				if s.runJob(ctx, "correlate.final", func() error { return s.Correlate(ctx, date) }) {
					lastFinalize = date
				}
			}
			if local.Format("15:04") == s.Config.Schedules.SummaryAt && lastSummary != date {
				if s.runJob(ctx, "summary", func() error { return s.GenerateSummary(ctx, date) }) {
					lastSummary = date
				}
			}
			if local.Format("15:04") == "03:00" && lastRetention != date && s.Config.RetentionDays > 0 {
				if n, err := s.DB.RedactEventsBefore(ctx, time.Now().AddDate(0, 0, -s.Config.RetentionDays)); err != nil {
					log.Printf("event retention: %v", err)
				} else {
					log.Printf("event retention redacted %d payloads", n)
					lastRetention = date
				}
			}
		}
	}
}
func (s *Server) runJob(ctx context.Context, name string, fn func() error) bool {
	s.jobMu.Lock()
	if s.runningJobs[name] {
		s.jobMu.Unlock()
		return false
	}
	s.runningJobs[name] = true
	s.jobMu.Unlock()
	defer func() { s.jobMu.Lock(); delete(s.runningJobs, name); s.jobMu.Unlock() }()
	id, err := s.DB.StartJob(ctx, name)
	if err != nil {
		log.Printf("start job %s: %v", name, err)
		return false
	}
	runErr := fn()
	if err := s.DB.FinishJob(ctx, id, runErr); err != nil {
		log.Printf("finish job %s: %v", name, err)
	}
	if runErr != nil {
		log.Printf("job %s: %v", name, runErr)
		return false
	}
	return true
}
func (s *Server) today() string { return s.local(time.Now()).Format("2006-01-02") }
func (s *Server) local(t time.Time) time.Time {
	loc, _ := time.LoadLocation(s.Config.Schedules.Timezone)
	return t.In(loc)
}

func (s *Server) loginPage(w http.ResponseWriter, _ *http.Request) {
	_ = s.templates.ExecuteTemplate(w, "login.html", nil)
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	candidate := argon2.IDKey([]byte(r.FormValue("password")), s.adminSalt, 1, 64*1024, 4, 32)
	if subtle.ConstantTimeCompare(candidate, s.adminHash) != 1 {
		w.WriteHeader(401)
		_ = s.templates.ExecuteTemplate(w, "login.html", map[string]string{"Error": "Senha inválida"})
		return
	}
	token := sessionToken()
	s.mu.Lock()
	s.sessions[token] = time.Now().Add(24 * time.Hour)
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "devlog_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: strings.HasPrefix(s.PublicURL, "https://"), MaxAge: 86400})
	http.Redirect(w, r, "/", 303)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("devlog_session"); err == nil {
		s.mu.Lock()
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "devlog_session", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", 303)
}
func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !s.sameOrigin(r) {
			http.Error(w, "cross-site request rejected", http.StatusForbidden)
			return
		}
		c, err := r.Cookie("devlog_session")
		if err != nil {
			http.Redirect(w, r, "/login", 303)
			return
		}
		s.mu.Lock()
		expiry, ok := s.sessions[c.Value]
		s.mu.Unlock()
		if !ok || time.Now().After(expiry) {
			http.Redirect(w, r, "/login", 303)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) sameOrigin(r *http.Request) bool {
	if s.PublicURL == "" {
		return true
	}
	expected, err := url.Parse(s.PublicURL)
	if err != nil {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Referer()
	}
	if origin == "" {
		return false
	}
	actual, err := url.Parse(origin)
	return err == nil && strings.EqualFold(actual.Host, expected.Host)
}
func (s *Server) timeline(w http.ResponseWriter, r *http.Request) {
	date := r.PathValue("date")
	if date == "" {
		date = s.today()
	}
	activities, err := s.DB.ActivitiesForDay(r.Context(), date)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	summary, _ := s.DB.LatestSummary(r.Context(), date)
	data := struct {
		Date, Notice string
		Activities   []domain.Activity
		Summary      *domain.Summary
	}{Date: date, Activities: activities}
	if summary.ID != "" {
		data.Summary = &summary
	}
	_ = s.templates.ExecuteTemplate(w, "index.html", data)
}
func (s *Server) devices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.DB.Devices(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	code, expires, active := s.pairing.Current()
	if !active {
		code = "expirado"
	}
	_ = s.templates.ExecuteTemplate(w, "devices.html", struct {
		PairingCode    string
		PairingExpires time.Time
		Devices        []domain.Device
	}{code, expires, devices})
}
func (s *Server) rotatePairingCode(w http.ResponseWriter, r *http.Request) {
	code := randomCode()
	s.pairing.Replace(code, 10*time.Minute)
	s.PairingCode = code
	http.Redirect(w, r, "/devices", 303)
}
func (s *Server) projects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.DB.Projects(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = s.templates.ExecuteTemplate(w, "projects.html", projects)
}
func (s *Server) jobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.DB.JobRuns(r.Context(), 50)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_ = s.templates.ExecuteTemplate(w, "jobs.html", jobs)
}
func (s *Server) settings(w http.ResponseWriter, _ *http.Request) {
	_ = s.templates.ExecuteTemplate(w, "settings.html", s.Config)
}
func (s *Server) revokeDevice(w http.ResponseWriter, r *http.Request) {
	if err := s.DB.RevokeDevice(r.Context(), r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	http.Redirect(w, r, "/devices", 303)
}
func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	remote := strings.TrimSpace(r.FormValue("remote"))
	id := slug(name)
	if id == "" || remote == "" {
		http.Error(w, "name and remote are required", 400)
		return
	}
	if err := s.DB.UpsertProject(r.Context(), domain.Project{ID: id, Name: name, CanonicalRemote: remote, Enabled: true, CreatedAt: time.Now().UTC()}); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/projects", 303)
}
func (s *Server) setProjectEnabled(w http.ResponseWriter, r *http.Request) {
	enabled := r.FormValue("enabled") == "true"
	if err := s.DB.SetProjectEnabled(r.Context(), r.PathValue("id"), enabled); err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	http.Redirect(w, r, "/projects", 303)
}
func (s *Server) activityStatus(w http.ResponseWriter, r *http.Request) {
	status := r.FormValue("status")
	if status != domain.ActivityConfirmed && status != domain.ActivityRejected {
		http.Error(w, "invalid status", 400)
		return
	}
	if err := s.DB.SetActivityStatus(r.Context(), r.PathValue("id"), status); err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	http.Redirect(w, r, r.Referer(), 303)
}
func (s *Server) editActivity(w http.ResponseWriter, r *http.Request) {
	if err := s.DB.UpdateActivity(r.Context(), r.PathValue("id"), strings.TrimSpace(r.FormValue("description")), strings.TrimSpace(r.FormValue("project"))); err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	http.Redirect(w, r, r.Referer(), 303)
}
func (s *Server) createActivity(w http.ResponseWriter, r *http.Request) {
	date := r.FormValue("date")
	if err := s.AddActivity(r.Context(), date, r.FormValue("description"), r.FormValue("project")); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	http.Redirect(w, r, "/days/"+date, 303)
}
func (s *Server) regenerate(w http.ResponseWriter, r *http.Request) {
	date := r.PathValue("date")
	if err := s.GenerateSummary(r.Context(), date); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/days/"+date, 303)
}
func (s *Server) confirmSummary(w http.ResponseWriter, r *http.Request) {
	if err := s.DB.ConfirmSummaryAndActivities(r.Context(), r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	http.Redirect(w, r, r.Referer(), 303)
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
func randomCode() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return strings.ToUpper(base64.RawURLEncoding.EncodeToString(b))
}
func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	dash := false
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
func sessionToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
