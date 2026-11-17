package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"devlog/internal/collector"
	"devlog/internal/store"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Warnings []string        `json:"warnings"`
	Error    struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func newTestServer(t *testing.T) (*Server, *time.Location) {
	t.Helper()
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	repo := store.NewRepository(filepath.Join(root, "data"), root)
	if _, err := repo.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 17, 18, 30, 0, 0, loc)
	return &Server{
		Repository: repo, Location: loc, DeviceName: "server", DefaultProject: "default",
		Tickets: regexp.MustCompile(collector.DefaultTicketPattern), now: func() time.Time { return now },
	}, loc
}

func connect(t *testing.T, server *Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.MCP().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func call(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) envelope {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("CallTool(%s) returned %T", name, result.Content[0])
	}
	var parsed envelope
	if err := json.Unmarshal([]byte(text.Text), &parsed); err != nil {
		t.Fatalf("CallTool(%s) returned %q: %v", name, text.Text, err)
	}
	return parsed
}

func TestGetActivityReturnsLocalTimesTicketsAndDevices(t *testing.T) {
	server, loc := newTestServer(t)
	at := func(hour, minute int) time.Time { return time.Date(2026, 8, 17, hour, minute, 0, 0, loc) }
	ended := at(12, 0)
	if _, err := server.Repository.UpsertEntries([]store.Entry{
		{Project: "widgets", Description: "feat: add widgets", Source: "commit:abcdef1234567890", Activity: &store.Activity{
			Kind: store.KindCommit, Collector: "git", OccurredAt: at(10, 42), Repo: "acme/widgets",
			Branch: "feature/PROJ-7-widgets", Tickets: []string{"PROJ-7"}, SHA: "abcdef1234567890", Device: "laptop",
		}},
		{Project: "widgets", Description: "Activity on branch feature/PROJ-7-widgets", Source: "git:branch:d:acme/widgets:feature/PROJ-7-widgets:2026-08-17", Activity: &store.Activity{
			Kind: store.KindBranchActivity, Collector: "git", OccurredAt: at(9, 5), EndedAt: &ended, Repo: "acme/widgets",
			Branch: "feature/PROJ-7-widgets", Tickets: []string{"PROJ-7"}, Changes: &store.Changes{Files: 2},
		}},
	}, loc); err != nil {
		t.Fatal(err)
	}
	if err := server.Repository.RecordCollectorRuns("laptop", map[string]store.CollectorStatus{
		"git": {LastRunAt: at(18, 15), LastSuccessAt: at(18, 15), CoveredTo: at(18, 15)},
	}); err != nil {
		t.Fatal(err)
	}
	refreshes := 0
	server.Refresh = func(context.Context) error { refreshes++; return nil }

	session := connect(t, server)
	result := call(t, session, "get_activity", map[string]any{"date": "2026-08-17"})
	if !result.OK {
		t.Fatalf("get_activity failed: %+v", result.Error)
	}
	var view ActivityView
	if err := json.Unmarshal(result.Data, &view); err != nil {
		t.Fatal(err)
	}
	if view.Timezone != "America/Los_Angeles" || len(view.Days) != 1 || view.Days[0].Weekday != "Monday" {
		t.Fatalf("view = %+v", view)
	}
	entries := view.Days[0].Entries
	if len(entries) != 2 || entries[0].Time != "09:05" || entries[0].End != "12:00" || entries[1].Time != "10:42" || entries[1].SHA != "abcdef123456" {
		t.Fatalf("entries = %+v", entries)
	}
	tickets := view.Days[0].Tickets
	if len(tickets) != 1 || tickets[0].Key != "PROJ-7" || tickets[0].First != "09:05" || tickets[0].Last != "12:00" || tickets[0].Entries != 2 {
		t.Fatalf("tickets = %+v", tickets)
	}
	if len(view.Devices) != 1 || view.Devices[0].Collectors["git"].CoveredUntil != "2026-08-17T18:15:00-07:00" {
		t.Fatalf("devices = %+v", view.Devices)
	}

	call(t, session, "get_activity", nil)
	if refreshes != 1 {
		t.Errorf("refreshes = %d, want one within a minute", refreshes)
	}
}

func TestGetActivityWarnsWhenRefreshFails(t *testing.T) {
	server, _ := newTestServer(t)
	server.Refresh = func(context.Context) error { return errors.New("offline") }
	result := call(t, connect(t, server), "get_activity", nil)
	if !result.OK || len(result.Warnings) != 1 {
		t.Fatalf("result = %+v, want local data with a warning", result)
	}
}

func TestGetActivityRejectsInvalidRanges(t *testing.T) {
	server, _ := newTestServer(t)
	session := connect(t, server)
	for _, arguments := range []map[string]any{
		{"date": "17/08/2026"},
		{"date": "2026-08-17", "from": "2026-08-16"},
		{"from": "2026-08-16"},
		{"from": "2026-08-17", "to": "2026-08-16"},
		{"from": "2026-07-01", "to": "2026-08-17"},
	} {
		if result := call(t, session, "get_activity", arguments); result.OK || result.Error.Code != "invalid_input" {
			t.Errorf("get_activity(%v) = %+v, want invalid_input", arguments, result)
		}
	}
}

func TestAddEntryStoresANoteAndPushesIt(t *testing.T) {
	server, loc := newTestServer(t)
	refreshes := 0
	server.Refresh = func(context.Context) error { refreshes++; return nil }
	session := connect(t, server)

	result := call(t, session, "add_entry", map[string]any{"description": "Refinement meeting for PROJ-240", "start": "14:00", "end": "15:00"})
	if !result.OK {
		t.Fatalf("add_entry failed: %+v", result.Error)
	}
	var entry EntryView
	if err := json.Unmarshal(result.Data, &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Kind != store.KindNote || entry.Time != "14:00" || entry.End != "15:00" || entry.Project != "default" || len(entry.Tickets) != 1 || entry.Tickets[0] != "PROJ-240" {
		t.Fatalf("entry = %+v", entry)
	}
	if refreshes != 1 {
		t.Errorf("refreshes = %d, want the note pushed", refreshes)
	}
	stored, err := server.Repository.Entries(time.Date(2026, 8, 17, 0, 0, 0, 0, loc))
	if err != nil || len(stored) != 1 || stored[0].Activity.Device != "server" {
		t.Fatalf("stored = %+v, %v", stored, err)
	}

	if result := call(t, session, "add_entry", map[string]any{"description": "Yesterday", "date": "2026-08-16"}); result.OK || result.Error.Code != "invalid_input" {
		t.Errorf("a note for another day without a start time was accepted: %+v", result)
	}
	if result := call(t, session, "add_entry", map[string]any{"description": "Backwards", "start": "15:00", "end": "14:00"}); result.OK {
		t.Errorf("a note ending before it starts was accepted: %+v", result)
	}
}

func TestRequireBearer(t *testing.T) {
	handler := RequireBearer("0123456789abcdef0123456789abcdef", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for header, want := range map[string]int{
		"":             http.StatusUnauthorized,
		"Bearer wrong": http.StatusUnauthorized,
		"Bearer 0123456789abcdef0123456789abcdef": http.StatusNoContent,
	} {
		request := httptest.NewRequest(http.MethodPost, EndpointPath, nil)
		if header != "" {
			request.Header.Set("Authorization", header)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != want {
			t.Errorf("Authorization %q: status %d, want %d", header, recorder.Code, want)
		}
	}
}
