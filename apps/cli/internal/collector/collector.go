// Package collector gathers development activity from local tools and remote
// services into DevLog entries, without rewriting it.
package collector

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"devlog/internal/store"

	// Collection runs on machines without Go's zone database, such as
	// Windows, where time.LoadLocation would otherwise fail.
	_ "time/tzdata"
)

// DefaultTicketPattern matches issue keys such as PROJ-236.
const DefaultTicketPattern = `\b[A-Z][A-Z0-9]+-[0-9]+\b`

// Window is the half-open time range [From, To) a collection covers.
type Window struct {
	From     time.Time
	To       time.Time
	Location *time.Location
}

// DayWindow covers the calendar day of day in loc.
func DayWindow(day time.Time, loc *time.Location) Window {
	from := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	return Window{From: from, To: from.AddDate(0, 0, 1), Location: loc}
}

// DaysWindow covers the calendar days from first to last, both included.
func DaysWindow(first, last time.Time, loc *time.Location) Window {
	window := DayWindow(first, loc)
	window.To = DayWindow(last, loc).To
	return window
}

// Contains reports whether t falls inside the window.
func (w Window) Contains(t time.Time) bool {
	return !t.Before(w.From) && t.Before(w.To)
}

// Day returns t's calendar day in the window's location as YYYY-MM-DD.
func (w Window) Day(t time.Time) string {
	return t.In(w.Location).Format("2006-01-02")
}

// Request is what a collector needs to run.
type Request struct {
	Window Window
	// DeviceID identifies this device in the sources of entries that only
	// it can see, such as branch activity.
	DeviceID string
}

// Collector gathers the entries of one source.
type Collector interface {
	Name() string
	Collect(ctx context.Context, request Request) ([]store.Entry, error)
}

// SkippedError reports that a collector's source is not available or not
// configured on this device. It is not a failure.
type SkippedError struct {
	Reason string
}

func (e *SkippedError) Error() string { return e.Reason }

// Skip returns a SkippedError with a formatted reason.
func Skip(format string, args ...any) error {
	return &SkippedError{Reason: fmt.Sprintf(format, args...)}
}

// Result is the outcome of one collector.
type Result struct {
	Collector string
	Entries   []store.Entry
	Skipped   string
	Err       error
}

// Options adjust the entries every collector returns.
type Options struct {
	DeviceID string
	// DeviceName labels the entries with the device that collected them.
	DeviceName string
	Tickets    *regexp.Regexp
}

// Run runs each collector over the window. Entries are labeled with the
// device, given the ticket keys found in their branch and first description
// line, and dropped when their activity falls outside the window. A collector
// that fails partway keeps the entries it returned with its error.
func Run(ctx context.Context, collectors []Collector, window Window, options Options) []Result {
	request := Request{Window: window, DeviceID: options.DeviceID}
	results := make([]Result, 0, len(collectors))
	for _, collector := range collectors {
		result := Result{Collector: collector.Name()}
		entries, err := collector.Collect(ctx, request)
		var skipped *SkippedError
		if errors.As(err, &skipped) {
			result.Skipped = skipped.Reason
			results = append(results, result)
			continue
		}
		result.Err = err
		for _, entry := range entries {
			if entry.Activity == nil || !window.Contains(entry.Activity.OccurredAt) {
				continue
			}
			activity := *entry.Activity
			if activity.Device == "" {
				activity.Device = options.DeviceName
			}
			activity.Tickets = append(slices.Clone(activity.Tickets), Tickets(options.Tickets, activity.Branch, firstLine(entry.Description))...)
			entry.Activity = &activity
			result.Entries = append(result.Entries, entry)
		}
		results = append(results, result)
	}
	return results
}

// Tickets returns the distinct ticket keys pattern finds in texts.
func Tickets(pattern *regexp.Regexp, texts ...string) []string {
	if pattern == nil {
		return nil
	}
	var found []string
	for _, text := range texts {
		for _, match := range pattern.FindAllString(text, -1) {
			if !slices.Contains(found, match) {
				found = append(found, match)
			}
		}
	}
	return found
}

// ShortRepoName turns "owner/repo" into "repo", the project name entries of
// that repository share.
func ShortRepoName(fullName string) string {
	if i := strings.LastIndex(fullName, "/"); i >= 0 {
		return fullName[i+1:]
	}
	return fullName
}

// Truncate shortens text to at most limit runes, marking the cut.
func Truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return strings.TrimSpace(string(runes[:limit])) + "…"
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}
