package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"devlog/internal/collector/gitlocal"
	"devlog/internal/config"
	"devlog/internal/database"
	"devlog/internal/syncapi"
)

type Agent struct {
	Config      config.Config
	DB          *database.DB
	Credentials syncapi.Credentials
	Interval    time.Duration
}

func (a *Agent) Run(ctx context.Context) error {
	if a.Interval == 0 {
		a.Interval = 5 * time.Minute
	}
	if err := a.Once(ctx); err != nil {
		log.Printf("agent cycle: %v", err)
	}
	ticker := time.NewTicker(a.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := a.Once(ctx); err != nil {
				log.Printf("agent cycle: %v", err)
			}
		}
	}
}
func (a *Agent) Once(ctx context.Context) error {
	for _, p := range a.Config.Projects {
		if !p.Enabled || p.Path == "" {
			continue
		}
		cursor, _ := a.DB.Cursor(ctx, "git", p.Path)
		collector := gitlocal.Collector{Root: config.Expand(p.Path), DeviceID: a.Credentials.DeviceID, ProjectID: p.ID}
		events, next, err := collector.Collect(ctx, cursor)
		if err != nil {
			return err
		}
		if len(events) > 0 {
			if _, err := a.DB.QueueEvents(ctx, events); err != nil {
				return err
			}
			if err := a.DB.SetCursor(ctx, "git", p.Path, next); err != nil {
				return err
			}
		}
	}
	return a.Sync(ctx)
}
func (a *Agent) Sync(ctx context.Context) error {
	if a.Config.Server.URL == "" {
		return nil
	}
	if !strings.HasPrefix(a.Config.Server.URL, "https://") && !a.Config.Server.AllowInsecure {
		return fmt.Errorf("server URL must use HTTPS")
	}
	pending, err := a.DB.PendingEvents(ctx, 200)
	if err != nil {
		return err
	}
	client := syncapi.Client{BaseURL: a.Config.Server.URL, Token: a.Credentials.Token}
	if len(pending) > 0 {
		result, err := client.Push(ctx, pending)
		if err != nil {
			return err
		}
		if err := a.DB.AckEvents(ctx, result.Acknowledged); err != nil {
			return err
		}
	}
	cursor, err := a.DB.SyncCursor(ctx)
	if err != nil {
		return err
	}
	changes, err := client.Changes(ctx, cursor)
	if err != nil {
		return err
	}
	dates := map[string]bool{}
	for _, change := range changes.Changes {
		if change.EntityType == "timeline" {
			dates[change.EntityID] = true
		}
	}
	for date := range dates {
		timeline, err := client.Timeline(ctx, date)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(timeline)
		if err != nil {
			return err
		}
		if err := a.DB.CacheTimeline(ctx, date, payload); err != nil {
			return err
		}
	}
	if err := a.DB.SetSyncCursor(ctx, changes.NextCursor); err != nil {
		return err
	}
	return nil
}
func Paths(home string) (dbPath, credentialsPath string) {
	base := filepath.Join(home, ".devlog")
	return filepath.Join(base, "devlog.db"), filepath.Join(base, "credentials.json")
}
func Open(home string) (*database.DB, error) {
	dbPath, _ := Paths(home)
	db, err := database.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open local database: %w", err)
	}
	return db, nil
}
