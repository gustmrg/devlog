package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (r *Repository) migrateLegacyUnlocked() (MigrationResult, error) {
	result := MigrationResult{}
	legacyEntries := filepath.Join(r.ConfigRoot, "entries")
	legacySummaries := filepath.Join(r.ConfigRoot, "summaries")
	entryFiles, err := os.ReadDir(legacyEntries)
	if err != nil && !os.IsNotExist(err) {
		return result, err
	}
	summaryFiles, err := os.ReadDir(legacySummaries)
	if err != nil && !os.IsNotExist(err) {
		return result, err
	}
	if len(entryFiles) == 0 && len(summaryFiles) == 0 {
		return result, nil
	}

	backup := filepath.Join(r.ConfigRoot, "backups", "migration-"+time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err := os.MkdirAll(backup, 0700); err != nil {
		return result, err
	}
	result.BackupPath = backup

	for _, item := range entryFiles {
		if item.IsDir() || filepath.Ext(item.Name()) != ".json" {
			continue
		}
		src := filepath.Join(legacyEntries, item.Name())
		if err := os.MkdirAll(filepath.Join(backup, "entries"), 0700); err != nil {
			return result, err
		}
		if err := copyFile(src, filepath.Join(backup, "entries", item.Name())); err != nil {
			return result, err
		}
		log, err := LoadDailyLog(src)
		if err != nil {
			return result, err
		}
		dateText := strings.TrimSuffix(item.Name(), filepath.Ext(item.Name()))
		date, err := time.Parse("2006-01-02", dateText)
		if err != nil {
			return result, fmt.Errorf("invalid legacy entry filename %s", item.Name())
		}
		dir := filepath.Join(r.Root, "entries", date.Format("2006-01-02"))
		if err := os.MkdirAll(dir, 0755); err != nil {
			return result, err
		}
		for _, entry := range log.Entries {
			entry.Id = normalizedEntryID(entry.Id, entry.Source)
			if entry.UpdatedAt.IsZero() {
				entry.UpdatedAt = entry.CreatedAt
			}
			data, err := marshalEntry(entry)
			if err != nil {
				return result, err
			}
			if err := atomicWrite(filepath.Join(dir, entry.Id+".json"), data, 0644); err != nil {
				return result, err
			}
			result.MigratedEntries++
		}
	}

	for _, item := range summaryFiles {
		if item.IsDir() || filepath.Ext(item.Name()) != ".md" {
			continue
		}
		if err := os.MkdirAll(filepath.Join(backup, "summaries"), 0700); err != nil {
			return result, err
		}
		src := filepath.Join(legacySummaries, item.Name())
		if err := copyFile(src, filepath.Join(backup, "summaries", item.Name())); err != nil {
			return result, err
		}
		if err := os.MkdirAll(filepath.Join(r.Root, "summaries"), 0755); err != nil {
			return result, err
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return result, err
		}
		if err := atomicWrite(filepath.Join(r.Root, "summaries", item.Name()), data, 0644); err != nil {
			return result, err
		}
		result.MigratedSummaries++
	}
	return result, nil
}
