package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// Activity kinds recorded by collectors.
const (
	KindCommit         = "commit"
	KindPR             = "pr"
	KindReview         = "review"
	KindComment        = "comment"
	KindBranchCreated  = "branch-created"
	KindBranchActivity = "branch-activity"
	KindWIP            = "wip"
	KindSession        = "session"
	KindNote           = "note"
)

// Activity describes when and where a collected entry happened. Manually
// added entries have none.
type Activity struct {
	Kind       string     `json:"kind"`
	Collector  string     `json:"collector"`
	OccurredAt time.Time  `json:"occurredAt"`
	EndedAt    *time.Time `json:"endedAt,omitempty"`
	// Repo is "owner/repo" when the repository has a GitHub-style remote and
	// the directory name otherwise.
	Repo      string   `json:"repo,omitempty"`
	Branch    string   `json:"branch,omitempty"`
	Tickets   []string `json:"tickets,omitempty"`
	Device    string   `json:"device,omitempty"`
	URL       string   `json:"url,omitempty"`
	SHA       string   `json:"sha,omitempty"`
	Number    int      `json:"number,omitempty"`
	Action    string   `json:"action,omitempty"`
	Directory string   `json:"directory,omitempty"`
	Changes   *Changes `json:"changes,omitempty"`
	// Count holds how many items an aggregated entry stands for, such as the
	// comments on a pull request or the prompts of a session.
	Count int `json:"count,omitempty"`
}

// Changes summarizes uncommitted or unpushed work in a repository.
type Changes struct {
	Files      int `json:"files,omitempty"`
	Insertions int `json:"insertions,omitempty"`
	Deletions  int `json:"deletions,omitempty"`
	Untracked  int `json:"untracked,omitempty"`
	Unpushed   int `json:"unpushed,omitempty"`
}

// Snapshot reports whether the activity describes a state that changes while
// the work goes on, so a later collection replaces it instead of filling it.
func (a *Activity) Snapshot() bool {
	switch a.Kind {
	case KindWIP, KindSession, KindBranchActivity:
		return true
	}
	return false
}

// SortTime is when the entry happened: its activity time for collected
// entries and its creation time for the others.
func (e Entry) SortTime() time.Time {
	if e.Activity != nil {
		return e.Activity.OccurredAt
	}
	return e.CreatedAt
}

// UpsertResult reports what UpsertEntries changed.
type UpsertResult struct {
	Added     []Entry
	Updated   []Entry
	Unchanged int
}

type storedEntry struct {
	entry Entry
	path  string
}

// UpsertEntries stores collected entries under the day their activity
// happened in loc. An entry whose source is already stored that day is merged
// into the stored one: snapshot kinds replace the stored values, and the other
// kinds only fill fields the stored entry lacks, so collecting the same item
// from several devices converges. Deleted entries stay deleted.
func (r *Repository) UpsertEntries(entries []Entry, loc *time.Location) (UpsertResult, error) {
	var result UpsertResult
	unlock, err := r.Lock()
	if err != nil {
		return result, err
	}
	defer unlock()

	days := map[string]map[string]*storedEntry{}
	load := func(day string) (map[string]*storedEntry, error) {
		if index, ok := days[day]; ok {
			return index, nil
		}
		stored, err := r.storedEntriesUnlocked(day)
		if err != nil {
			return nil, err
		}
		index := make(map[string]*storedEntry, len(stored))
		for i := range stored {
			if stored[i].entry.Source != "" {
				index[stored[i].entry.Source] = &stored[i]
			}
		}
		days[day] = index
		return index, nil
	}

	now := time.Now()
	for _, incoming := range entries {
		if incoming.Source == "" || incoming.Activity == nil {
			return result, fmt.Errorf("collected entry %q needs a source and an activity", incoming.Description)
		}
		incoming = normalizeCollected(incoming, loc, now)
		day := incoming.Activity.OccurredAt.Format("2006-01-02")
		index, err := load(day)
		if err != nil {
			return result, err
		}
		existing := index[incoming.Source]
		for _, alias := range sourceAliases(incoming.Source) {
			if existing == nil {
				existing = index[alias]
			}
		}
		if existing == nil {
			path := filepath.Join(r.Root, "entries", day, incoming.Id+".json")
			if err := writeEntry(path, incoming); err != nil {
				return result, err
			}
			index[incoming.Source] = &storedEntry{entry: incoming, path: path}
			result.Added = append(result.Added, incoming)
			continue
		}
		if existing.entry.DeletedAt != nil {
			result.Unchanged++
			continue
		}
		merged := mergeEntry(existing.entry, incoming)
		if sameEntry(merged, existing.entry) {
			result.Unchanged++
			continue
		}
		merged.UpdatedAt = now
		if err := writeEntry(existing.path, merged); err != nil {
			return result, err
		}
		existing.entry = merged
		result.Updated = append(result.Updated, merged)
	}
	return result, nil
}

func normalizeCollected(entry Entry, loc *time.Location, now time.Time) Entry {
	activity := *entry.Activity
	activity.OccurredAt = activity.OccurredAt.In(loc)
	if activity.EndedAt != nil {
		ended := activity.EndedAt.In(loc)
		activity.EndedAt = &ended
	}
	activity.Tickets = uniqueSorted(activity.Tickets)
	entry.Activity = &activity
	entry.Id = EntryID(entry.Source)
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = entry.CreatedAt
	return entry
}

// sourceAliases lists older sources that identify the same item, so entries
// imported by earlier versions are not stored twice.
func sourceAliases(source string) []string {
	if sha, ok := strings.CutPrefix(source, "commit:"); ok {
		return []string{"github:commit:" + sha}
	}
	return nil
}

func mergeEntry(stored, incoming Entry) Entry {
	merged := stored
	if incoming.Activity.Snapshot() {
		merged.Project = incoming.Project
		merged.Description = incoming.Description
		merged.Tags = incoming.Tags
		activity := *incoming.Activity
		merged.Activity = &activity
		return merged
	}
	fill := func(value *string, candidate string) {
		if *value == "" {
			*value = candidate
		}
	}
	fill(&merged.Project, incoming.Project)
	fill(&merged.Description, incoming.Description)
	if stored.Activity == nil {
		activity := *incoming.Activity
		merged.Activity = &activity
		return merged
	}
	activity := *stored.Activity
	in := incoming.Activity
	fill(&activity.Repo, in.Repo)
	fill(&activity.Branch, in.Branch)
	fill(&activity.URL, in.URL)
	fill(&activity.SHA, in.SHA)
	fill(&activity.Action, in.Action)
	fill(&activity.Directory, in.Directory)
	if activity.Number == 0 {
		activity.Number = in.Number
	}
	if activity.EndedAt == nil {
		activity.EndedAt = in.EndedAt
	}
	if activity.Changes == nil {
		activity.Changes = in.Changes
	}
	activity.Tickets = uniqueSorted(append(slices.Clone(activity.Tickets), in.Tickets...))
	merged.Activity = &activity
	return merged
}

func sameEntry(a, b Entry) bool {
	a.UpdatedAt, b.UpdatedAt = time.Time{}, time.Time{}
	first, err := json.Marshal(a)
	if err != nil {
		return false
	}
	second, err := json.Marshal(b)
	return err == nil && string(first) == string(second)
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	sorted := slices.Clone(values)
	sort.Strings(sorted)
	return slices.Compact(sorted)
}

func writeEntry(path string, entry Entry) error {
	data, err := marshalEntry(entry)
	if err != nil {
		return fmt.Errorf("could not encode entry %s: %w", entry.Id, err)
	}
	if err := atomicWrite(path, data, 0644); err != nil {
		return fmt.Errorf("could not write entry %s: %w", entry.Id, err)
	}
	return nil
}

// storedEntriesUnlocked reads every entry of a day, deleted ones included,
// with the file each one came from.
func (r *Repository) storedEntriesUnlocked(day string) ([]storedEntry, error) {
	dir := filepath.Join(r.Root, "entries", day)
	items, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	stored := make([]storedEntry, 0, len(items))
	for _, item := range items {
		if item.IsDir() || filepath.Ext(item.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, item.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("could not read entry %s: %w", item.Name(), err)
		}
		var entry Entry
		if err := json.Unmarshal(data, &entry); err != nil {
			return nil, fmt.Errorf("could not parse entry %s: %w", item.Name(), err)
		}
		stored = append(stored, storedEntry{entry: entry, path: path})
	}
	return stored, nil
}

// DeviceStatus records when each collector last ran on one device, so readers
// can tell missing activity from a device that did not collect.
type DeviceStatus struct {
	DeviceID   string                     `json:"deviceId"`
	Name       string                     `json:"name,omitempty"`
	UpdatedAt  time.Time                  `json:"updatedAt"`
	Collectors map[string]CollectorStatus `json:"collectors"`
}

// CollectorStatus is the outcome of a collector's latest run on a device.
type CollectorStatus struct {
	LastRunAt     time.Time `json:"lastRunAt"`
	LastSuccessAt time.Time `json:"lastSuccessAt,omitzero"`
	// CoveredFrom and CoveredTo bound the window of the last successful run.
	CoveredFrom time.Time `json:"coveredFrom,omitzero"`
	CoveredTo   time.Time `json:"coveredTo,omitzero"`
	Entries     int       `json:"entries"`
	Skipped     string    `json:"skipped,omitempty"`
	Error       string    `json:"error,omitempty"`
}

func (r *Repository) devicePath(deviceID string) string {
	return filepath.Join(r.Root, "devices", deviceID+".json")
}

// RecordCollectorRuns updates this device's status with the given runs. A
// failed run keeps the time and window of the last successful one.
func (r *Repository) RecordCollectorRuns(name string, runs map[string]CollectorStatus) error {
	deviceID, err := DeviceID(r.ConfigRoot)
	if err != nil {
		return fmt.Errorf("could not read the device ID: %w", err)
	}
	unlock, err := r.Lock()
	if err != nil {
		return err
	}
	defer unlock()

	path := r.devicePath(deviceID)
	status := DeviceStatus{DeviceID: deviceID}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &status); err != nil {
			return fmt.Errorf("could not parse device status %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("could not read device status %s: %w", path, err)
	}
	if status.Collectors == nil {
		status.Collectors = map[string]CollectorStatus{}
	}
	status.Name = name
	status.UpdatedAt = time.Now()
	for collector, run := range runs {
		if run.Error != "" {
			previous := status.Collectors[collector]
			run.LastSuccessAt, run.CoveredFrom, run.CoveredTo = previous.LastSuccessAt, previous.CoveredFrom, previous.CoveredTo
		}
		status.Collectors[collector] = run
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("could not encode device status: %w", err)
	}
	if err := atomicWrite(path, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("could not write device status: %w", err)
	}
	return nil
}

// DeviceStatuses returns the collection status of every device.
func (r *Repository) DeviceStatuses() ([]DeviceStatus, error) {
	unlock, err := r.Lock()
	if err != nil {
		return nil, err
	}
	defer unlock()

	dir := filepath.Join(r.Root, "devices")
	items, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("could not list device statuses: %w", err)
	}
	var statuses []DeviceStatus
	for _, item := range items {
		if item.IsDir() || filepath.Ext(item.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, item.Name()))
		if err != nil {
			return nil, fmt.Errorf("could not read device status %s: %w", item.Name(), err)
		}
		var status DeviceStatus
		if err := json.Unmarshal(data, &status); err != nil {
			return nil, fmt.Errorf("could not parse device status %s: %w", item.Name(), err)
		}
		statuses = append(statuses, status)
	}
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Name+statuses[i].DeviceID < statuses[j].Name+statuses[j].DeviceID
	})
	return statuses, nil
}
