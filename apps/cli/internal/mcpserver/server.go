// Package mcpserver serves DevLog's collected activity to AI agents over the
// Model Context Protocol.
package mcpserver

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"devlog/internal/collector"
	"devlog/internal/store"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	EndpointPath       = "/mcp"
	MinTokenLength     = 32
	sessionTimeout     = 30 * time.Minute
	shutdownTimeout    = 5 * time.Second
	refreshInterval    = time.Minute
	maxDays            = 31
	maxEntries         = 500
	maxDescriptionSize = 800
)

const instructions = `Read the development activity DevLog collected from Git, GitHub, Claude Code, and opencode on the user's devices, and record work that left no trace.

- get_activity returns, for each day, the collected entries in time order with their repository, branch, ticket keys, and times in the user's time zone, plus the tickets seen that day. Entries are raw facts: commit messages and pull request titles as their authors wrote them, often in English, and the titles of coding agent sessions. Put them in your own words when summarizing, and never add work that is not in the entries or that the user did not tell you about.
- Kinds: commit; pr (opened or merged); review; comment (on an issue or pull request, counted per day); branch-created; branch-activity (when a branch was worked on in a repository, with the uncommitted and unpushed changes found when it was collected); session (a Claude Code or opencode session with its time window and number of prompts); note (added by the user).
- Group work by ticket key when there is one, and by repository and branch otherwise. Uncommitted or unpushed changes, or a pull request opened but not merged, mean the work is still in progress; a merged pull request means it was delivered.
- devices tells when each device last collected each source and up to when. Before concluding that a period had no work, check that the devices had collected it: a device that has not collected since then may hold the missing work, so say so instead of guessing.
- add_entry records something the user says they did, such as a meeting or an investigation, at the time they give. Ask for the time when it is not clear.
- Successful results are {"ok":true,"data":...} and may carry "warnings"; failures are {"ok":false,"error":{"code":"...","message":"..."}}.`

// Server answers tool calls from one DevLog data directory.
type Server struct {
	Repository *store.Repository
	Location   *time.Location
	// Refresh synchronizes the data with the other devices. It is nil when
	// remote synchronization is off.
	Refresh        func(ctx context.Context) error
	DeviceName     string
	DefaultProject string
	Tickets        *regexp.Regexp
	Version        string

	mu          sync.Mutex
	lastRefresh time.Time
	now         func() time.Time
}

type activityInput struct {
	Date string `json:"date,omitempty" jsonschema:"day as YYYY-MM-DD; defaults to today"`
	From string `json:"from,omitempty" jsonschema:"first day of a range as YYYY-MM-DD, used with to"`
	To   string `json:"to,omitempty" jsonschema:"last day of a range as YYYY-MM-DD, used with from"`
}

type addEntryInput struct {
	Description string   `json:"description" jsonschema:"what the user did, in their words"`
	Project     string   `json:"project,omitempty" jsonschema:"project or repository name; defaults to the configured default project"`
	Tickets     []string `json:"tickets,omitempty" jsonschema:"ticket keys the work belongs to, such as PROJ-236"`
	Date        string   `json:"date,omitempty" jsonschema:"day as YYYY-MM-DD; defaults to today"`
	Start       string   `json:"start,omitempty" jsonschema:"start time as 24-hour HH:mm; defaults to now, and is required for another day"`
	End         string   `json:"end,omitempty" jsonschema:"end time as 24-hour HH:mm"`
}

// ActivityView is what get_activity returns.
type ActivityView struct {
	Timezone  string       `json:"timezone"`
	Days      []DayView    `json:"days"`
	Devices   []DeviceView `json:"devices"`
	Truncated bool         `json:"truncated,omitempty"`
}

type DayView struct {
	Date    string       `json:"date"`
	Weekday string       `json:"weekday"`
	Tickets []TicketView `json:"tickets,omitempty"`
	Entries []EntryView  `json:"entries"`
}

type TicketView struct {
	Key      string   `json:"key"`
	Repos    []string `json:"repos,omitempty"`
	Branches []string `json:"branches,omitempty"`
	First    string   `json:"first"`
	Last     string   `json:"last"`
	Entries  int      `json:"entries"`
}

type EntryView struct {
	Time        string         `json:"time"`
	End         string         `json:"end,omitempty"`
	Kind        string         `json:"kind"`
	Action      string         `json:"action,omitempty"`
	Project     string         `json:"project,omitempty"`
	Repo        string         `json:"repo,omitempty"`
	Branch      string         `json:"branch,omitempty"`
	Tickets     []string       `json:"tickets,omitempty"`
	Description string         `json:"description"`
	Number      int            `json:"number,omitempty"`
	SHA         string         `json:"sha,omitempty"`
	Count       int            `json:"count,omitempty"`
	Changes     *store.Changes `json:"changes,omitempty"`
	URL         string         `json:"url,omitempty"`
	Collector   string         `json:"collector,omitempty"`
	Device      string         `json:"device,omitempty"`
}

type DeviceView struct {
	Name       string                   `json:"name"`
	Collectors map[string]CollectorView `json:"collectors"`
}

type CollectorView struct {
	LastRunAt     string `json:"lastRunAt"`
	LastSuccessAt string `json:"lastSuccessAt,omitempty"`
	CoveredUntil  string `json:"coveredUntil,omitempty"`
	Skipped       string `json:"skipped,omitempty"`
	Error         string `json:"error,omitempty"`
}

// toolError is a failure reported to the agent with a stable code.
type toolError struct {
	code, message string
}

func (e *toolError) Error() string { return e.message }

func invalid(format string, args ...any) error {
	return &toolError{code: "invalid_input", message: fmt.Sprintf(format, args...)}
}

// MCP returns the protocol server with DevLog's tools.
func (s *Server) MCP() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "devlog", Title: "DevLog", Version: s.Version}, &mcp.ServerOptions{Instructions: instructions})
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_activity", Title: "Get activity",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPointer(false)},
		Description: "Return the activity collected for a day or a range of days (at most 31): entries in time order with kind, repository, branch, ticket keys, description, and local times, the tickets seen each day, and when each device last collected.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input activityInput) (*mcp.CallToolResult, any, error) {
		return s.respond(ctx, true, func(ctx context.Context) (any, []string, error) { return s.activity(input) })
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "add_entry", Title: "Add entry",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPointer(false), OpenWorldHint: boolPointer(false)},
		Description: "Record work the user describes, such as a meeting or an investigation, as a note at the given time. Returns the stored entry.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input addEntryInput) (*mcp.CallToolResult, any, error) {
		return s.respond(ctx, false, func(ctx context.Context) (any, []string, error) { return s.addEntry(ctx, input) })
	})
	return server
}

// respond runs one operation and wraps its outcome in DevLog's JSON envelope.
// Reads first refresh the data from the other devices, at most once a minute.
func (s *Server) respond(ctx context.Context, refresh bool, operation func(context.Context) (any, []string, error)) (*mcp.CallToolResult, any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var warnings []string
	if refresh {
		warnings = s.refresh(ctx, false)
	}
	data, more, err := operation(ctx)
	warnings = append(warnings, more...)
	if err != nil {
		code, message := "operation_failed", err.Error()
		var known *toolError
		if errors.As(err, &known) {
			code = known.code
		}
		body, marshalErr := json.Marshal(map[string]any{"ok": false, "error": map[string]string{"code": code, "message": message}})
		if marshalErr != nil {
			return nil, nil, marshalErr
		}
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}}, nil, nil
	}
	envelope := map[string]any{"ok": true, "data": data}
	if len(warnings) > 0 {
		envelope["warnings"] = warnings
	}
	return nil, envelope, nil
}

func (s *Server) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *Server) refresh(ctx context.Context, force bool) []string {
	if s.Refresh == nil || !force && s.clock().Sub(s.lastRefresh) < refreshInterval {
		return nil
	}
	if err := s.Refresh(ctx); err != nil {
		return []string{"could not synchronize with the other devices; the activity may be incomplete: " + err.Error()}
	}
	s.lastRefresh = s.clock()
	return nil
}

func (s *Server) parseDay(flag, value string) (time.Time, error) {
	day, err := time.ParseInLocation("2006-01-02", value, s.Location)
	if err != nil {
		return time.Time{}, invalid("invalid %s %q: expected YYYY-MM-DD", flag, value)
	}
	return day, nil
}

func (s *Server) activity(input activityInput) (any, []string, error) {
	first := s.clock().In(s.Location)
	last := first
	switch {
	case input.Date != "" && (input.From != "" || input.To != ""):
		return nil, nil, invalid("give either date or from and to")
	case input.Date != "":
		day, err := s.parseDay("date", input.Date)
		if err != nil {
			return nil, nil, err
		}
		first, last = day, day
	case input.From != "" || input.To != "":
		if input.From == "" || input.To == "" {
			return nil, nil, invalid("from and to must be given together")
		}
		var err error
		if first, err = s.parseDay("from", input.From); err != nil {
			return nil, nil, err
		}
		if last, err = s.parseDay("to", input.To); err != nil {
			return nil, nil, err
		}
		if last.Before(first) {
			return nil, nil, invalid("to must not be earlier than from")
		}
	}
	window := collector.DaysWindow(first, last, s.Location)
	if days := int(window.To.Sub(window.From).Hours()/24 + 0.5); days > maxDays {
		return nil, nil, invalid("ask for at most %d days at a time", maxDays)
	}

	view := ActivityView{Timezone: s.Location.String(), Days: []DayView{}, Devices: []DeviceView{}}
	total := 0
	for day := window.From; day.Before(window.To); day = day.AddDate(0, 0, 1) {
		entries, err := s.Repository.Entries(day)
		if err != nil {
			return nil, nil, err
		}
		dayView := DayView{Date: day.Format("2006-01-02"), Weekday: day.Weekday().String(), Entries: []EntryView{}}
		for _, entry := range entries {
			if total == maxEntries {
				view.Truncated = true
				break
			}
			dayView.Entries = append(dayView.Entries, s.entryView(entry))
			total++
		}
		dayView.Tickets = ticketViews(dayView.Entries)
		view.Days = append(view.Days, dayView)
	}

	statuses, err := s.Repository.DeviceStatuses()
	if err != nil {
		return nil, nil, err
	}
	for _, status := range statuses {
		device := DeviceView{Name: status.Name, Collectors: map[string]CollectorView{}}
		if device.Name == "" {
			device.Name = status.DeviceID
		}
		for name, run := range status.Collectors {
			device.Collectors[name] = CollectorView{
				LastRunAt:     s.stamp(run.LastRunAt),
				LastSuccessAt: s.stamp(run.LastSuccessAt),
				CoveredUntil:  s.stamp(run.CoveredTo),
				Skipped:       run.Skipped,
				Error:         run.Error,
			}
		}
		view.Devices = append(view.Devices, device)
	}
	var warnings []string
	if view.Truncated {
		warnings = append(warnings, fmt.Sprintf("only the first %d entries are included; ask for fewer days", maxEntries))
	}
	return view, warnings, nil
}

func (s *Server) stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(s.Location).Format(time.RFC3339)
}

func (s *Server) entryView(entry store.Entry) EntryView {
	view := EntryView{
		Project:     entry.Project,
		Description: collector.Truncate(entry.Description, maxDescriptionSize),
	}
	activity := entry.Activity
	if activity == nil {
		view.Kind = store.KindNote
		view.Time = entry.CreatedAt.In(s.Location).Format("15:04")
		return view
	}
	view.Time = activity.OccurredAt.In(s.Location).Format("15:04")
	if activity.EndedAt != nil {
		view.End = activity.EndedAt.In(s.Location).Format("15:04")
	}
	view.Kind, view.Action, view.Repo, view.Branch = activity.Kind, activity.Action, activity.Repo, activity.Branch
	view.Tickets, view.Number, view.Count, view.Changes = activity.Tickets, activity.Number, activity.Count, activity.Changes
	view.URL, view.Collector, view.Device = activity.URL, activity.Collector, activity.Device
	if len(activity.SHA) > 12 {
		view.SHA = activity.SHA[:12]
	} else {
		view.SHA = activity.SHA
	}
	return view
}

func ticketViews(entries []EntryView) []TicketView {
	byKey := map[string]*TicketView{}
	var keys []string
	for _, entry := range entries {
		for _, key := range entry.Tickets {
			ticket := byKey[key]
			if ticket == nil {
				ticket = &TicketView{Key: key, First: entry.Time, Last: entry.Time}
				byKey[key] = ticket
				keys = append(keys, key)
			}
			ticket.Entries++
			if entry.Repo != "" && !slices.Contains(ticket.Repos, entry.Repo) {
				ticket.Repos = append(ticket.Repos, entry.Repo)
			}
			if entry.Branch != "" && !slices.Contains(ticket.Branches, entry.Branch) {
				ticket.Branches = append(ticket.Branches, entry.Branch)
			}
			end := entry.Time
			if entry.End > end {
				end = entry.End
			}
			if entry.Time < ticket.First {
				ticket.First = entry.Time
			}
			if end > ticket.Last {
				ticket.Last = end
			}
		}
	}
	sort.Strings(keys)
	views := make([]TicketView, 0, len(keys))
	for _, key := range keys {
		views = append(views, *byKey[key])
	}
	return views
}

func (s *Server) addEntry(ctx context.Context, input addEntryInput) (any, []string, error) {
	description := strings.TrimSpace(input.Description)
	if description == "" {
		return nil, nil, invalid("description is required")
	}
	now := s.clock().In(s.Location)
	day := now
	if input.Date != "" {
		var err error
		if day, err = s.parseDay("date", input.Date); err != nil {
			return nil, nil, err
		}
	}
	sameDay := day.Format("2006-01-02") == now.Format("2006-01-02")
	at := func(field, value string) (time.Time, error) {
		clock, err := time.Parse("15:04", value)
		if err != nil {
			return time.Time{}, invalid("invalid %s %q: expected 24-hour HH:mm", field, value)
		}
		return time.Date(day.Year(), day.Month(), day.Day(), clock.Hour(), clock.Minute(), 0, 0, s.Location), nil
	}
	start := now
	switch {
	case input.Start != "":
		var err error
		if start, err = at("start", input.Start); err != nil {
			return nil, nil, err
		}
	case !sameDay:
		return nil, nil, invalid("give the start time for a day other than today")
	}
	var end *time.Time
	if input.End != "" {
		parsed, err := at("end", input.End)
		if err != nil {
			return nil, nil, err
		}
		if !parsed.After(start) {
			return nil, nil, invalid("end must be after start")
		}
		end = &parsed
	}
	project := strings.TrimSpace(input.Project)
	if project == "" {
		project = s.DefaultProject
	}
	tickets := append(slices.Clone(input.Tickets), collector.Tickets(s.Tickets, description)...)
	entry := store.Entry{
		Project:     project,
		Description: description,
		Tags:        []string{"note"},
		Source:      "note:" + uuid.NewString(),
		Activity: &store.Activity{
			Kind: store.KindNote, Collector: "manual", OccurredAt: start, EndedAt: end,
			Tickets: tickets, Device: s.DeviceName,
		},
	}
	result, err := s.Repository.UpsertEntries([]store.Entry{entry}, s.Location)
	if err != nil {
		return nil, nil, err
	}
	warnings := s.refresh(ctx, true)
	return s.entryView(result.Added[0]), warnings, nil
}

// ServeHTTP serves the protocol as Streamable HTTP at EndpointPath until ctx
// ends. Every request must carry the bearer token.
func ServeHTTP(ctx context.Context, address, token string, server *mcp.Server, log io.Writer) error {
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		SessionTimeout: sessionTimeout,
		// Reverse proxies such as `tailscale serve` reach the loopback listener
		// with their public Host header, which the SDK's DNS-rebinding check
		// rejects. The bearer token already stops requests a hostile page could
		// make through a rebound name.
		DisableLocalhostProtection: true,
	})
	mux := http.NewServeMux()
	mux.Handle(EndpointPath, RequireBearer(token, handler))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("could not listen on %s: %w", address, err)
	}
	httpServer := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	fmt.Fprintf(log, "DevLog MCP server listening on http://%s%s\n", listener.Addr(), EndpointPath)
	if err := httpServer.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("MCP server stopped: %w", err)
	}
	<-shutdownDone
	return nil
}

// RequireBearer rejects requests without the expected bearer token.
func RequireBearer(token string, next http.Handler) http.Handler {
	expected := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), expected) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="devlog"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func boolPointer(value bool) *bool { return &value }
