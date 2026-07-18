package entry

import (
	"os"
	"testing"
	"time"

	"devlog/internal/agent"
	"github.com/spf13/viper"
)

func TestAddQueuesManualEventInCanonicalDatabase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	viper.Reset()
	viper.Set("defaults.project", "devlog")
	cmd := NewAddCmd()
	cmd.SetArgs([]string{"Implemented automatic capture", "--tags", "git,sync"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	db, err := agent.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	events, err := db.EventsForDay(t.Context(), time.Now().Format("2006-01-02"), time.Local)
	if err != nil || len(events) != 1 {
		t.Fatalf("events=%d err=%v", len(events), err)
	}
	pending, err := db.PendingEvents(t.Context(), 10)
	if err != nil || len(pending) != 1 || pending[0].Sequence != 1 {
		t.Fatalf("pending=%v err=%v", pending, err)
	}
	if _, err := os.Stat(home + "/.devlog/devlog.db"); err != nil {
		t.Fatal(err)
	}
}
