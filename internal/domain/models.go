package domain

import (
	"encoding/json"
	"time"
)

type Event struct {
	ID               string          `json:"id"`
	SourceType       string          `json:"sourceType"`
	SourceInstanceID string          `json:"sourceInstanceId,omitempty"`
	ExternalID       string          `json:"externalId,omitempty"`
	DeviceID         string          `json:"deviceId,omitempty"`
	ProjectID        string          `json:"projectId,omitempty"`
	Kind             string          `json:"kind"`
	OccurredAt       time.Time       `json:"occurredAt"`
	ObservedAt       time.Time       `json:"observedAt"`
	ReceivedAt       time.Time       `json:"receivedAt,omitempty"`
	Payload          json.RawMessage `json:"payload,omitempty"`
	Fingerprint      string          `json:"fingerprint"`
	Sequence         int64           `json:"sequence,omitempty"`
}

type Evidence struct {
	EventID string `json:"eventId"`
	Label   string `json:"label"`
}

type Activity struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"projectId,omitempty"`
	Description string     `json:"description"`
	StartedAt   time.Time  `json:"startedAt"`
	EndedAt     time.Time  `json:"endedAt"`
	Status      string     `json:"status"`
	Confidence  string     `json:"confidence"`
	Evidence    []Evidence `json:"evidence,omitempty"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

type Summary struct {
	ID        string    `json:"id"`
	Date      string    `json:"date"`
	Revision  int       `json:"revision"`
	Content   string    `json:"content"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

type Project struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	CanonicalRemote string    `json:"canonicalRemote,omitempty"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"createdAt"`
}

type Device struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	LastSeenAt time.Time  `json:"lastSeenAt"`
	CreatedAt  time.Time  `json:"createdAt"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
}
type Change struct {
	Sequence   int64     `json:"sequence"`
	EntityType string    `json:"entityType"`
	EntityID   string    `json:"entityId"`
	ChangedAt  time.Time `json:"changedAt"`
}
type JobRun struct {
	ID         int64      `json:"id"`
	JobType    string     `json:"jobType"`
	Status     string     `json:"status"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Error      string     `json:"error,omitempty"`
}

const (
	ActivityDraft     = "draft"
	ActivityConfirmed = "confirmed"
	ActivityRejected  = "rejected"

	ConfidenceLow    = "low"
	ConfidenceMedium = "medium"
	ConfidenceHigh   = "high"
)
