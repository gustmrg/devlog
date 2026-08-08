package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/viper"
)

const DataVersion = "1"

type Repository struct {
	Root       string
	ConfigRoot string
}

type MigrationResult struct {
	MigratedEntries   int
	MigratedSummaries int
	BackupPath        string
	AlreadyCurrent    bool
}

func OpenRepository() (*Repository, error) {
	repo, _, err := openRepository()
	return repo, err
}

func MigrateRepository() (*Repository, MigrationResult, error) {
	return openRepository()
}

func openRepository() (*Repository, MigrationResult, error) {
	configRoot, err := ConfigPath()
	if err != nil {
		return nil, MigrationResult{}, err
	}
	root := viper.GetString("storage.path")
	if root == "" {
		root = filepath.Join(configRoot, "data")
	} else {
		root, err = expandPath(root)
		if err != nil {
			return nil, MigrationResult{}, err
		}
	}
	repo := &Repository{Root: root, ConfigRoot: configRoot}
	if unsafeDataRoot(root, configRoot) {
		return nil, MigrationResult{}, fmt.Errorf("storage.path must not contain ~/.devlog configuration; choose a dedicated data directory")
	}
	result, err := repo.EnsureLayout()
	if err != nil {
		return nil, MigrationResult{}, err
	}
	return repo, result, nil
}

func NewRepository(root, configRoot string) *Repository {
	return &Repository{Root: root, ConfigRoot: configRoot}
}

func (r *Repository) EnsureLayout() (MigrationResult, error) {
	if err := os.MkdirAll(r.Root, 0755); err != nil {
		return MigrationResult{}, err
	}
	unlock, err := r.Lock()
	if err != nil {
		return MigrationResult{}, err
	}
	defer unlock()
	return r.ensureLayoutUnlocked()
}

func (r *Repository) ensureLayoutUnlocked() (MigrationResult, error) {
	versionPath := filepath.Join(r.Root, ".devlog-version")
	if data, err := os.ReadFile(versionPath); err == nil {
		if strings.TrimSpace(string(data)) != DataVersion {
			return MigrationResult{}, fmt.Errorf("unsupported devlog data version %q", strings.TrimSpace(string(data)))
		}
		return MigrationResult{AlreadyCurrent: true}, nil
	} else if !os.IsNotExist(err) {
		return MigrationResult{}, err
	}

	result, err := r.migrateLegacyUnlocked()
	if err != nil {
		return MigrationResult{}, err
	}
	if err := os.MkdirAll(filepath.Join(r.Root, "entries"), 0755); err != nil {
		return MigrationResult{}, err
	}
	if err := os.MkdirAll(filepath.Join(r.Root, "summaries"), 0755); err != nil {
		return MigrationResult{}, err
	}
	if err := atomicWrite(versionPath, []byte(DataVersion+"\n"), 0644); err != nil {
		return MigrationResult{}, err
	}
	if err := r.ensureGitignore(); err != nil {
		return MigrationResult{}, err
	}
	return result, nil
}

func (r *Repository) Migrate() (MigrationResult, error) {
	return r.EnsureLayout()
}

func (r *Repository) Lock() (func(), error) {
	if err := os.MkdirAll(r.Root, 0755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(r.Root, ".devlog.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		file.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}

func (r *Repository) AddEntries(date time.Time, entries []Entry) ([]Entry, error) {
	unlock, err := r.Lock()
	if err != nil {
		return nil, err
	}
	defer unlock()

	existing, err := r.entriesUnlocked(date)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]bool, len(existing))
	sources := make(map[string]bool, len(existing))
	for _, entry := range existing {
		ids[entry.Id] = true
		if entry.Source != "" {
			sources[entry.Source] = true
		}
	}

	dir := filepath.Join(r.Root, "entries", date.Format("2006-01-02"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	var added []Entry
	for _, entry := range entries {
		entry.Id = normalizedEntryID(entry.Id, entry.Source)
		if ids[entry.Id] || entry.Source != "" && sources[entry.Source] {
			continue
		}
		if entry.UpdatedAt.IsZero() {
			entry.UpdatedAt = entry.CreatedAt
		}
		data, err := json.MarshalIndent(entry, "", "  ")
		if err != nil {
			return added, err
		}
		if err := atomicWrite(filepath.Join(dir, entry.Id+".json"), append(data, '\n'), 0644); err != nil {
			return added, err
		}
		ids[entry.Id] = true
		if entry.Source != "" {
			sources[entry.Source] = true
		}
		added = append(added, entry)
	}
	return added, nil
}

func (r *Repository) Entries(date time.Time) ([]Entry, error) {
	unlock, err := r.Lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	return r.entriesUnlocked(date)
}

func (r *Repository) entriesUnlocked(date time.Time) ([]Entry, error) {
	dir := filepath.Join(r.Root, "entries", date.Format("2006-01-02"))
	items, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(items))
	for _, item := range items {
		if item.IsDir() || filepath.Ext(item.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, item.Name()))
		if err != nil {
			return nil, err
		}
		var entry Entry
		if err := json.Unmarshal(data, &entry); err != nil {
			return nil, fmt.Errorf("error parsing entry %s: %w", item.Name(), err)
		}
		if entry.DeletedAt != nil {
			continue
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].CreatedAt.Equal(entries[j].CreatedAt) {
			return entries[i].Id < entries[j].Id
		}
		return entries[i].CreatedAt.Before(entries[j].CreatedAt)
	})
	return entries, nil
}

func (r *Repository) SummaryPath(date time.Time) string {
	return filepath.Join(r.Root, "summaries", date.Format("2006-01-02")+".md")
}

func (r *Repository) SaveSummary(summary Summary) error {
	unlock, err := r.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	if summary.GeneratedAt.IsZero() {
		summary.GeneratedAt = time.Now().UTC()
	}
	if summary.DeviceID == "" {
		summary.DeviceID, _ = DeviceID(r.ConfigRoot)
	}
	if err := os.MkdirAll(filepath.Join(r.Root, "summaries"), 0755); err != nil {
		return err
	}
	return saveSummaryAtomic(r.SummaryPath(summary.Date), summary)
}

func (r *Repository) LoadSummary(date time.Time) (Summary, error) {
	return LoadSummary(r.SummaryPath(date))
}

func (r *Repository) SummaryFiles() ([]string, error) {
	dir := filepath.Join(r.Root, "summaries")
	items, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, item := range items {
		if !item.IsDir() && strings.HasSuffix(item.Name(), ".md") {
			paths = append(paths, filepath.Join(dir, item.Name()))
		}
	}
	return paths, nil
}

func EntryID(source string) string {
	if source == "" {
		return uuid.NewString()
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(source)).String()
}

func normalizedEntryID(id, source string) string {
	if source != "" {
		return EntryID(source)
	}
	if id == "" {
		return EntryID("")
	}
	if len(id) <= 128 && id != "." && id != ".." && strings.IndexFunc(id, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.')
	}) == -1 {
		return id
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("legacy-entry:"+id)).String()
}

func DeviceID(configRoot string) (string, error) {
	path := filepath.Join(configRoot, "device-id")
	if data, err := os.ReadFile(path); err == nil {
		return strings.TrimSpace(string(data)), nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	id := uuid.NewString()
	if err := atomicWrite(path, []byte(id+"\n"), 0600); err != nil {
		return "", err
	}
	return id, nil
}

func (r *Repository) ensureGitignore() error {
	path := filepath.Join(r.Root, ".gitignore")
	const required = ".devlog.lock\n*.tmp\n"
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return atomicWrite(path, []byte(required), 0644)
	}
	if err != nil {
		return err
	}
	content := string(data)
	changed := false
	for _, line := range strings.Split(strings.TrimSpace(required), "\n") {
		if !containsLine(content, line) {
			if content != "" && !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			content += line + "\n"
			changed = true
		}
	}
	if changed {
		return atomicWrite(path, []byte(content), 0644)
	}
	return nil
}

func containsLine(content, wanted string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == wanted {
			return true
		}
	}
	return false
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".devlog-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return filepath.Abs(path)
}

func unsafeDataRoot(root, configRoot string) bool {
	root, _ = filepath.Abs(root)
	configRoot, _ = filepath.Abs(configRoot)
	if pathContains(root, configRoot) {
		return true
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	if resolved, err := filepath.EvalSymlinks(configRoot); err == nil {
		configRoot = resolved
	}
	return pathContains(root, configRoot)
}

func pathContains(root, path string) bool {
	if root == path {
		return true
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	return errors.Join(copyErr, closeErr)
}
