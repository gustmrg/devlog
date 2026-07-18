package syncapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"devlog/internal/database"
	"devlog/internal/domain"
	"github.com/google/uuid"
)

type Credentials struct {
	DeviceID string `json:"deviceId"`
	Token    string `json:"token"`
}
type PairRequest struct {
	Code string `json:"code"`
	Name string `json:"name"`
}
type PairResponse struct {
	DeviceID string `json:"deviceId"`
	Token    string `json:"token"`
}
type PushRequest struct {
	Events []domain.Event `json:"events"`
}
type PushResponse struct {
	Accepted     int      `json:"accepted"`
	Acknowledged []string `json:"acknowledged"`
}
type TimelineResponse struct {
	Activities []domain.Activity `json:"activities"`
	Summary    *domain.Summary   `json:"summary,omitempty"`
}
type ChangesResponse struct {
	Changes    []domain.Change `json:"changes"`
	NextCursor int64           `json:"nextCursor"`
}

type Handler struct {
	DB          *database.DB
	PairingCode string
	Pairing     *Pairing
	OnEvents    func([]domain.Event)
}
type Pairing struct {
	mu      sync.Mutex
	code    string
	expires time.Time
	used    bool
}

func NewPairing(code string, ttl time.Duration) *Pairing {
	return &Pairing{code: code, expires: time.Now().Add(ttl)}
}
func (p *Pairing) Consume(code string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.used || time.Now().After(p.expires) || code == "" || code != p.code {
		return false
	}
	p.used = true
	return true
}
func (p *Pairing) Replace(code string, ttl time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.code = code
	p.expires = time.Now().Add(ttl)
	p.used = false
}
func (p *Pairing) Current() (string, time.Time, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.code, p.expires, !p.used && time.Now().Before(p.expires)
}

func (h Handler) Pair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req PairRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	valid := req.Code != "" && req.Code == h.PairingCode
	if h.Pairing != nil {
		valid = h.Pairing.Consume(req.Code)
	}
	if !valid {
		http.Error(w, "invalid pairing code", http.StatusUnauthorized)
		return
	}
	token, err := randomToken()
	if err != nil {
		http.Error(w, "could not create token", 500)
		return
	}
	id := uuid.NewString()
	if err := h.DB.CreateDevice(r.Context(), id, req.Name, HashToken(token)); err != nil {
		http.Error(w, "could not register device", 500)
		return
	}
	writeJSON(w, PairResponse{DeviceID: id, Token: token})
}

func (h Handler) Push(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	deviceID, ok := h.authenticate(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	var req PushRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	if len(req.Events) > 500 {
		http.Error(w, "batch exceeds 500 events", 400)
		return
	}
	for i := range req.Events {
		req.Events[i].DeviceID = deviceID
		req.Events[i].ReceivedAt = time.Now().UTC()
		if req.Events[i].ProjectID != "" {
			remote := ""
			var metadata struct {
				Remote string `json:"remote"`
			}
			_ = json.Unmarshal(req.Events[i].Payload, &metadata)
			remote = metadata.Remote
			_ = h.DB.UpsertProject(r.Context(), domain.Project{ID: req.Events[i].ProjectID, Name: req.Events[i].ProjectID, CanonicalRemote: remote, Enabled: true, CreatedAt: time.Now().UTC()})
		}
	}
	n, err := h.DB.InsertEvents(r.Context(), req.Events)
	if err != nil {
		http.Error(w, "could not store events", 500)
		return
	}
	ids := make([]string, len(req.Events))
	for i, e := range req.Events {
		ids[i] = e.ID
	}
	if n > 0 && h.OnEvents != nil {
		h.OnEvents(req.Events)
	}
	writeJSON(w, PushResponse{Accepted: n, Acknowledged: ids})
}
func (h Handler) Timeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	if _, ok := h.authenticate(r); !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	date := r.URL.Query().Get("date")
	if _, err := time.Parse("2006-01-02", date); err != nil {
		http.Error(w, "invalid date", 400)
		return
	}
	activities, err := h.DB.ActivitiesForDay(r.Context(), date)
	if err != nil {
		http.Error(w, "could not load timeline", 500)
		return
	}
	summary, err := h.DB.LatestSummary(r.Context(), date)
	var summaryPtr *domain.Summary
	if err == nil {
		summaryPtr = &summary
	}
	writeJSON(w, TimelineResponse{Activities: activities, Summary: summaryPtr})
}
func (h Handler) Changes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	if _, ok := h.authenticate(r); !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	cursor, _ := strconv.ParseInt(r.URL.Query().Get("cursor"), 10, 64)
	changes, err := h.DB.ChangesAfter(r.Context(), cursor, 500)
	if err != nil {
		http.Error(w, "could not load changes", 500)
		return
	}
	next := cursor
	if len(changes) > 0 {
		next = changes[len(changes)-1].Sequence
	}
	writeJSON(w, ChangesResponse{Changes: changes, NextCursor: next})
}

func (h Handler) authenticate(r *http.Request) (string, bool) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		return "", false
	}
	id, err := h.DB.AuthenticateDevice(r.Context(), HashToken(token))
	return id, err == nil
}

type Client struct {
	BaseURL, Token string
	HTTP           *http.Client
}

func (c Client) Pair(ctx context.Context, code, name string) (PairResponse, error) {
	var out PairResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/devices/pair", "", PairRequest{Code: code, Name: name}, &out)
	return out, err
}
func (c Client) Push(ctx context.Context, events []domain.Event) (PushResponse, error) {
	var out PushResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/sync/events", c.Token, PushRequest{Events: events}, &out)
	return out, err
}
func (c Client) Timeline(ctx context.Context, date string) (TimelineResponse, error) {
	var out TimelineResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/timeline?date="+url.QueryEscape(date), c.Token, nil, &out)
	return out, err
}
func (c Client) Changes(ctx context.Context, cursor int64) (ChangesResponse, error) {
	var out ChangesResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/sync/changes?cursor="+strconv.FormatInt(cursor, 10), c.Token, nil, &out)
	return out, err
}
func (c Client) do(ctx context.Context, method, path, token string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("server returned %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func SaveCredentials(path string, c Credentials) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(path, b, 0o600)
}
func LoadCredentials(path string) (Credentials, error) {
	var c Credentials
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(b, &c)
	return c, err
}
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
