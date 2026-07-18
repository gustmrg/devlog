package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"devlog/internal/domain"
	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
	queueMu sync.Mutex
}

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	d := &DB{DB: db}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := d.ExecContext(ctx, "PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;"); err != nil {
		d.Close()
		return nil, err
	}
	if err := d.Migrate(ctx); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) Migrate(ctx context.Context) error {
	_, err := d.ExecContext(ctx, schema)
	return err
}

func (d *DB) InsertEvents(ctx context.Context, events []domain.Event) (int, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO events
		(id, source_type, source_instance_id, external_id, device_id, project_id, kind,
		 occurred_at, observed_at, received_at, payload, fingerprint, sequence)
		VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	inserted := 0
	for _, e := range events {
		received := e.ReceivedAt
		if received.IsZero() {
			received = time.Now().UTC()
		}
		res, err := stmt.ExecContext(ctx, e.ID, e.SourceType, e.SourceInstanceID, e.ExternalID,
			e.DeviceID, e.ProjectID, e.Kind, dbTime(e.OccurredAt), dbTime(e.ObservedAt), dbTime(received),
			[]byte(e.Payload), e.Fingerprint, e.Sequence)
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			inserted += int(n)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return inserted, nil
}

func (d *DB) QueueEvents(ctx context.Context, events []domain.Event) (int, error) {
	d.queueMu.Lock()
	defer d.queueMu.Unlock()
	sequence, err := d.localSequence(ctx)
	if err != nil {
		return 0, err
	}
	for i := range events {
		if events[i].Sequence == 0 {
			sequence++
			events[i].Sequence = sequence
		}
	}
	n, err := d.InsertEvents(ctx, events)
	if err != nil {
		return 0, err
	}
	for _, e := range events {
		if _, err := d.ExecContext(ctx, `INSERT OR IGNORE INTO outbox(event_id) VALUES(?)`, e.ID); err != nil {
			return 0, err
		}
	}
	if err := d.setLocalSequence(ctx, sequence); err != nil {
		return 0, err
	}
	return n, nil
}
func (d *DB) localSequence(ctx context.Context) (int64, error) {
	var value int64
	err := d.QueryRowContext(ctx, `SELECT CAST(value AS INTEGER) FROM local_state WHERE key='event_sequence'`).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return value, err
}
func (d *DB) setLocalSequence(ctx context.Context, value int64) error {
	_, err := d.ExecContext(ctx, `INSERT INTO local_state(key,value) VALUES('event_sequence',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, fmt.Sprint(value))
	return err
}

func (d *DB) PendingEvents(ctx context.Context, limit int) ([]domain.Event, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := d.QueryContext(ctx, `SELECT e.id,e.source_type,e.source_instance_id,e.external_id,e.device_id,COALESCE(e.project_id,''),e.kind,e.occurred_at,e.observed_at,e.received_at,e.payload,e.fingerprint,e.sequence FROM events e JOIN outbox o ON o.event_id=e.id ORDER BY e.sequence,e.observed_at LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Event
	for rows.Next() {
		var e domain.Event
		var payload []byte
		var occurred, observed, received string
		if err := rows.Scan(&e.ID, &e.SourceType, &e.SourceInstanceID, &e.ExternalID, &e.DeviceID, &e.ProjectID, &e.Kind, &occurred, &observed, &received, &payload, &e.Fingerprint, &e.Sequence); err != nil {
			return nil, err
		}
		e.OccurredAt, _ = parseDBTime(occurred)
		e.ObservedAt, _ = parseDBTime(observed)
		e.ReceivedAt, _ = parseDBTime(received)
		e.Payload = payload
		out = append(out, e)
	}
	return out, rows.Err()
}

func (d *DB) AckEvents(ctx context.Context, ids []string) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `DELETE FROM outbox WHERE event_id=?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) EventsForDay(ctx context.Context, date string, location *time.Location) ([]domain.Event, error) {
	day, err := time.ParseInLocation("2006-01-02", date, location)
	if err != nil {
		return nil, err
	}
	rows, err := d.QueryContext(ctx, `SELECT id, source_type, source_instance_id, external_id,
		device_id, COALESCE(project_id,''), kind, occurred_at, observed_at, received_at,
		payload, fingerprint, sequence FROM events e WHERE occurred_at >= ? AND occurred_at < ? AND NOT EXISTS (SELECT 1 FROM activity_evidence ae JOIN activities a ON a.id=ae.activity_id WHERE ae.event_id=e.id AND a.status IN ('confirmed','rejected')) ORDER BY occurred_at`,
		dbTime(day), dbTime(day.AddDate(0, 0, 1)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Event
	for rows.Next() {
		var e domain.Event
		var payload []byte
		var occurred, observed, received string
		if err := rows.Scan(&e.ID, &e.SourceType, &e.SourceInstanceID, &e.ExternalID, &e.DeviceID,
			&e.ProjectID, &e.Kind, &occurred, &observed, &received, &payload,
			&e.Fingerprint, &e.Sequence); err != nil {
			return nil, err
		}
		e.OccurredAt, _ = parseDBTime(occurred)
		e.ObservedAt, _ = parseDBTime(observed)
		e.ReceivedAt, _ = parseDBTime(received)
		e.Payload = payload
		out = append(out, e)
	}
	return out, rows.Err()
}

func (d *DB) UpsertProject(ctx context.Context, p domain.Project) error {
	_, err := d.ExecContext(ctx, `INSERT INTO projects(id,name,canonical_remote,enabled,created_at)
		VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,
		canonical_remote=CASE WHEN excluded.canonical_remote='' THEN projects.canonical_remote ELSE excluded.canonical_remote END, enabled=excluded.enabled`,
		p.ID, p.Name, p.CanonicalRemote, p.Enabled, dbTime(p.CreatedAt))
	return err
}

func (d *DB) Projects(ctx context.Context) ([]domain.Project, error) {
	rows, err := d.QueryContext(ctx, `SELECT id,name,canonical_remote,enabled,created_at FROM projects ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Project
	for rows.Next() {
		var p domain.Project
		var created string
		if err := rows.Scan(&p.ID, &p.Name, &p.CanonicalRemote, &p.Enabled, &created); err != nil {
			return nil, err
		}
		p.CreatedAt, _ = parseDBTime(created)
		out = append(out, p)
	}
	return out, rows.Err()
}
func (d *DB) SetProjectEnabled(ctx context.Context, id string, enabled bool) error {
	res, err := d.ExecContext(ctx, `UPDATE projects SET enabled=? WHERE id=?`, enabled, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (d *DB) ReplaceActivitiesForDay(ctx context.Context, date string, activities []domain.Activity) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM activity_evidence WHERE activity_id IN (SELECT id FROM activities WHERE day=? AND status='draft')`, date); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM activities WHERE day=? AND status='draft'`, date); err != nil {
		return err
	}
	for _, a := range activities {
		_, err := tx.ExecContext(ctx, `INSERT INTO activities(id,day,project_id,description,started_at,ended_at,status,confidence,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			a.ID, date, a.ProjectID, a.Description, dbTime(a.StartedAt), dbTime(a.EndedAt), a.Status, a.Confidence, dbTime(a.UpdatedAt))
		if err != nil {
			return err
		}
		for _, ev := range a.Evidence {
			if _, err := tx.ExecContext(ctx, `INSERT INTO activity_evidence(activity_id,event_id,label) VALUES(?,?,?)`, a.ID, ev.EventID, ev.Label); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return d.recordChange(ctx, "timeline", date)
}

func (d *DB) ActivitiesForDay(ctx context.Context, date string) ([]domain.Activity, error) {
	rows, err := d.QueryContext(ctx, `SELECT id,COALESCE(project_id,''),description,started_at,ended_at,status,confidence,updated_at FROM activities WHERE day=? AND status!='rejected' ORDER BY started_at`, date)
	if err != nil {
		return nil, err
	}
	var out []domain.Activity
	for rows.Next() {
		var a domain.Activity
		var started, ended, updated string
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.Description, &started, &ended, &a.Status, &a.Confidence, &updated); err != nil {
			return nil, err
		}
		a.StartedAt, _ = parseDBTime(started)
		a.EndedAt, _ = parseDBTime(ended)
		a.UpdatedAt, _ = parseDBTime(updated)
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for i := range out {
		evRows, err := d.QueryContext(ctx, `SELECT event_id,label FROM activity_evidence WHERE activity_id=?`, out[i].ID)
		if err != nil {
			return nil, err
		}
		for evRows.Next() {
			var e domain.Evidence
			if err := evRows.Scan(&e.EventID, &e.Label); err != nil {
				evRows.Close()
				return nil, err
			}
			out[i].Evidence = append(out[i].Evidence, e)
		}
		if err := evRows.Err(); err != nil {
			evRows.Close()
			return nil, err
		}
		evRows.Close()
	}
	return out, nil
}

func (d *DB) SetActivityStatus(ctx context.Context, id, status string) error {
	res, err := d.ExecContext(ctx, `UPDATE activities SET status=?,updated_at=? WHERE id=?`, status, dbTime(time.Now()), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return d.recordActivityChange(ctx, id)
}

func (d *DB) CreateActivity(ctx context.Context, date string, a domain.Activity) error {
	_, err := d.ExecContext(ctx, `INSERT INTO activities(id,day,project_id,description,started_at,ended_at,status,confidence,updated_at) VALUES(?,?,NULLIF(?,''),?,?,?,?,?,?)`, a.ID, date, a.ProjectID, a.Description, dbTime(a.StartedAt), dbTime(a.EndedAt), a.Status, a.Confidence, dbTime(a.UpdatedAt))
	if err != nil {
		return err
	}
	return d.recordChange(ctx, "timeline", date)
}

func (d *DB) UpdateActivity(ctx context.Context, id, description, projectID string) error {
	res, err := d.ExecContext(ctx, `UPDATE activities SET description=?,project_id=NULLIF(?,''),status='confirmed',updated_at=? WHERE id=?`, description, projectID, dbTime(time.Now()), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return d.recordActivityChange(ctx, id)
}

func (d *DB) SaveSummary(ctx context.Context, s domain.Summary) error {
	_, err := d.ExecContext(ctx, `INSERT INTO summaries(id,day,revision,content,status,created_at) VALUES(?,?,?,?,?,?)`, s.ID, s.Date, s.Revision, s.Content, s.Status, dbTime(s.CreatedAt))
	if err != nil {
		return err
	}
	return d.recordChange(ctx, "timeline", s.Date)
}

func (d *DB) LatestSummary(ctx context.Context, date string) (domain.Summary, error) {
	var s domain.Summary
	var created string
	err := d.QueryRowContext(ctx, `SELECT id,day,revision,content,status,created_at FROM summaries WHERE day=? ORDER BY revision DESC LIMIT 1`, date).Scan(&s.ID, &s.Date, &s.Revision, &s.Content, &s.Status, &created)
	if err == nil {
		s.CreatedAt, _ = parseDBTime(created)
	}
	return s, err
}

func (d *DB) SetSummaryStatus(ctx context.Context, id, status string) error {
	res, err := d.ExecContext(ctx, `UPDATE summaries SET status=? WHERE id=?`, status, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	var date string
	if err := d.QueryRowContext(ctx, `SELECT day FROM summaries WHERE id=?`, id).Scan(&date); err != nil {
		return err
	}
	return d.recordChange(ctx, "timeline", date)
}
func (d *DB) ConfirmSummaryAndActivities(ctx context.Context, id string) error {
	var date string
	if err := d.QueryRowContext(ctx, `SELECT day FROM summaries WHERE id=?`, id).Scan(&date); err != nil {
		return err
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE summaries SET status='confirmed' WHERE id=?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE activities SET status='confirmed',updated_at=? WHERE day=? AND status='draft'`, dbTime(time.Now()), date); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return d.recordChange(ctx, "timeline", date)
}

func (d *DB) UpdateSummaryContent(ctx context.Context, id, content string) error {
	res, err := d.ExecContext(ctx, `UPDATE summaries SET content=?,status='draft' WHERE id=?`, content, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	var date string
	if err := d.QueryRowContext(ctx, `SELECT day FROM summaries WHERE id=?`, id).Scan(&date); err != nil {
		return err
	}
	return d.recordChange(ctx, "timeline", date)
}

func (d *DB) NextSummaryRevision(ctx context.Context, date string) (int, error) {
	var n int
	err := d.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision),0)+1 FROM summaries WHERE day=?`, date).Scan(&n)
	return n, err
}

func (d *DB) CreateDevice(ctx context.Context, id, name, tokenHash string) error {
	_, err := d.ExecContext(ctx, `INSERT INTO devices(id,name,token_hash,last_seen_at,created_at,revoked_at) VALUES(?,?,?,?,?,NULL)`, id, name, tokenHash, dbTime(time.Now()), dbTime(time.Now()))
	return err
}

func (d *DB) Devices(ctx context.Context) ([]domain.Device, error) {
	rows, err := d.QueryContext(ctx, `SELECT id,name,last_seen_at,created_at,revoked_at FROM devices ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Device
	for rows.Next() {
		var item domain.Device
		var lastSeen, created string
		var revoked sql.NullString
		if err := rows.Scan(&item.ID, &item.Name, &lastSeen, &created, &revoked); err != nil {
			return nil, err
		}
		item.LastSeenAt, _ = parseDBTime(lastSeen)
		item.CreatedAt, _ = parseDBTime(created)
		if revoked.Valid {
			parsed, _ := parseDBTime(revoked.String)
			item.RevokedAt = &parsed
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (d *DB) RevokeDevice(ctx context.Context, id string) error {
	res, err := d.ExecContext(ctx, `UPDATE devices SET revoked_at=? WHERE id=? AND revoked_at IS NULL`, dbTime(time.Now()), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (d *DB) AuthenticateDevice(ctx context.Context, tokenHash string) (string, error) {
	var id string
	err := d.QueryRowContext(ctx, `SELECT id FROM devices WHERE token_hash=? AND revoked_at IS NULL`, tokenHash).Scan(&id)
	if err == nil {
		_, _ = d.ExecContext(ctx, `UPDATE devices SET last_seen_at=? WHERE id=?`, dbTime(time.Now()), id)
	}
	return id, err
}

func (d *DB) SetCursor(ctx context.Context, source, key, value string) error {
	_, err := d.ExecContext(ctx, `INSERT INTO collection_cursors(source_type,cursor_key,cursor_value,updated_at) VALUES(?,?,?,?) ON CONFLICT(source_type,cursor_key) DO UPDATE SET cursor_value=excluded.cursor_value,updated_at=excluded.updated_at`, source, key, value, dbTime(time.Now()))
	return err
}

func (d *DB) Backup(ctx context.Context, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("backup destination already exists: %s", path)
	}
	_, err := d.ExecContext(ctx, `VACUUM INTO ?`, path)
	return err
}
func (d *DB) RedactEventsBefore(ctx context.Context, before time.Time) (int64, error) {
	res, err := d.ExecContext(ctx, `UPDATE events SET payload=NULL WHERE occurred_at < ? AND payload IS NOT NULL`, dbTime(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
func (d *DB) StartJob(ctx context.Context, jobType string) (int64, error) {
	res, err := d.ExecContext(ctx, `INSERT INTO job_runs(job_type,status,started_at) VALUES(?,'running',?)`, jobType, dbTime(time.Now()))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}
func (d *DB) FinishJob(ctx context.Context, id int64, jobErr error) error {
	status := "completed"
	message := ""
	if jobErr != nil {
		status = "failed"
		message = jobErr.Error()
	}
	_, err := d.ExecContext(ctx, `UPDATE job_runs SET status=?,finished_at=?,error=? WHERE id=?`, status, dbTime(time.Now()), message, id)
	return err
}
func (d *DB) JobRuns(ctx context.Context, limit int) ([]domain.JobRun, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.QueryContext(ctx, `SELECT id,job_type,status,started_at,finished_at,COALESCE(error,'') FROM job_runs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.JobRun
	for rows.Next() {
		var item domain.JobRun
		var started string
		var finished sql.NullString
		if err := rows.Scan(&item.ID, &item.JobType, &item.Status, &started, &finished, &item.Error); err != nil {
			return nil, err
		}
		item.StartedAt, _ = parseDBTime(started)
		if finished.Valid {
			parsed, _ := parseDBTime(finished.String)
			item.FinishedAt = &parsed
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func (d *DB) recordActivityChange(ctx context.Context, id string) error {
	var date string
	if err := d.QueryRowContext(ctx, `SELECT day FROM activities WHERE id=?`, id).Scan(&date); err != nil {
		return err
	}
	return d.recordChange(ctx, "timeline", date)
}
func (d *DB) recordChange(ctx context.Context, entityType, entityID string) error {
	_, err := d.ExecContext(ctx, `INSERT INTO change_log(entity_type,entity_id,changed_at) VALUES(?,?,?)`, entityType, entityID, dbTime(time.Now()))
	return err
}
func (d *DB) ChangesAfter(ctx context.Context, cursor int64, limit int) ([]domain.Change, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := d.QueryContext(ctx, `SELECT sequence,entity_type,entity_id,changed_at FROM change_log WHERE sequence>? ORDER BY sequence LIMIT ?`, cursor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Change
	for rows.Next() {
		var item domain.Change
		var changed string
		if err := rows.Scan(&item.Sequence, &item.EntityType, &item.EntityID, &changed); err != nil {
			return nil, err
		}
		item.ChangedAt, _ = parseDBTime(changed)
		out = append(out, item)
	}
	return out, rows.Err()
}
func (d *DB) CacheTimeline(ctx context.Context, date string, payload []byte) error {
	_, err := d.ExecContext(ctx, `INSERT INTO timeline_cache(day,payload,updated_at) VALUES(?,?,?) ON CONFLICT(day) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at`, date, payload, dbTime(time.Now()))
	return err
}
func (d *DB) CachedTimeline(ctx context.Context, date string) ([]byte, error) {
	var payload []byte
	err := d.QueryRowContext(ctx, `SELECT payload FROM timeline_cache WHERE day=?`, date).Scan(&payload)
	return payload, err
}
func (d *DB) SetSyncCursor(ctx context.Context, cursor int64) error {
	_, err := d.ExecContext(ctx, `INSERT INTO local_state(key,value) VALUES('sync_cursor',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, fmt.Sprint(cursor))
	return err
}
func (d *DB) SyncCursor(ctx context.Context) (int64, error) {
	var cursor int64
	err := d.QueryRowContext(ctx, `SELECT CAST(value AS INTEGER) FROM local_state WHERE key='sync_cursor'`).Scan(&cursor)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return cursor, err
}

func (d *DB) Cursor(ctx context.Context, source, key string) (string, error) {
	var v string
	err := d.QueryRowContext(ctx, `SELECT cursor_value FROM collection_cursors WHERE source_type=? AND cursor_key=?`, source, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func EncodePayload(v any) json.RawMessage         { b, _ := json.Marshal(v); return b }
func dbTime(t time.Time) string                   { return t.UTC().Format(time.RFC3339Nano) }
func parseDBTime(value string) (time.Time, error) { return time.Parse(time.RFC3339Nano, value) }

const schema = `
CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
INSERT OR IGNORE INTO schema_migrations(version,applied_at) VALUES(1,CURRENT_TIMESTAMP);
CREATE TABLE IF NOT EXISTS devices(id TEXT PRIMARY KEY,name TEXT NOT NULL,token_hash TEXT UNIQUE NOT NULL,last_seen_at TEXT NOT NULL,created_at TEXT NOT NULL,revoked_at TEXT);
CREATE TABLE IF NOT EXISTS projects(id TEXT PRIMARY KEY,name TEXT NOT NULL,canonical_remote TEXT NOT NULL DEFAULT '',enabled INTEGER NOT NULL DEFAULT 0,created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS events(id TEXT PRIMARY KEY,source_type TEXT NOT NULL,source_instance_id TEXT NOT NULL DEFAULT '',external_id TEXT NOT NULL DEFAULT '',device_id TEXT NOT NULL DEFAULT '',project_id TEXT REFERENCES projects(id),kind TEXT NOT NULL,occurred_at TEXT NOT NULL,observed_at TEXT NOT NULL,received_at TEXT NOT NULL,payload BLOB,fingerprint TEXT NOT NULL,sequence INTEGER NOT NULL DEFAULT 0,UNIQUE(source_type,source_instance_id,external_id),UNIQUE(fingerprint));
CREATE INDEX IF NOT EXISTS idx_events_occurred ON events(occurred_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_events_device_sequence ON events(device_id,sequence) WHERE device_id!='' AND sequence>0;
CREATE TABLE IF NOT EXISTS activities(id TEXT PRIMARY KEY,day TEXT NOT NULL,project_id TEXT REFERENCES projects(id),description TEXT NOT NULL,started_at TEXT NOT NULL,ended_at TEXT NOT NULL,status TEXT NOT NULL CHECK(status IN ('draft','confirmed','rejected')),confidence TEXT NOT NULL CHECK(confidence IN ('low','medium','high')),updated_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS idx_activities_day ON activities(day);
CREATE TABLE IF NOT EXISTS activity_evidence(activity_id TEXT NOT NULL REFERENCES activities(id) ON DELETE CASCADE,event_id TEXT NOT NULL REFERENCES events(id),label TEXT NOT NULL,PRIMARY KEY(activity_id,event_id));
CREATE TABLE IF NOT EXISTS summaries(id TEXT PRIMARY KEY,day TEXT NOT NULL,revision INTEGER NOT NULL,content TEXT NOT NULL,status TEXT NOT NULL,created_at TEXT NOT NULL,UNIQUE(day,revision));
CREATE TABLE IF NOT EXISTS collection_cursors(source_type TEXT NOT NULL,cursor_key TEXT NOT NULL,cursor_value TEXT NOT NULL,updated_at TEXT NOT NULL,PRIMARY KEY(source_type,cursor_key));
CREATE TABLE IF NOT EXISTS job_runs(id INTEGER PRIMARY KEY AUTOINCREMENT,job_type TEXT NOT NULL,status TEXT NOT NULL,started_at TEXT NOT NULL,finished_at TEXT,error TEXT);
CREATE TABLE IF NOT EXISTS outbox(event_id TEXT PRIMARY KEY REFERENCES events(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS change_log(sequence INTEGER PRIMARY KEY AUTOINCREMENT,entity_type TEXT NOT NULL,entity_id TEXT NOT NULL,changed_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS timeline_cache(day TEXT PRIMARY KEY,payload BLOB NOT NULL,updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS local_state(key TEXT PRIMARY KEY,value TEXT NOT NULL);
`
