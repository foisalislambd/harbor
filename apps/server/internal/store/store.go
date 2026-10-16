package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"harbor/internal/id"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrPaused   = errors.New("paused")
	ErrQuota    = errors.New("quota")
)

type Store struct {
	db    *sql.DB
	blobs string
}

type User struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Password  string `json:"-"`
}

type Project struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	Description    string `json:"description"`
	Status         string `json:"status"`
	RetentionCount int    `json:"retention_count"`
	RetentionDays  int    `json:"retention_days"`
	QuotaBytes     int64  `json:"quota_bytes"`
	UsedBytes      int64  `json:"used_bytes"`
	BackupCount    int    `json:"backup_count"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type APIKey struct {
	ID         string  `json:"id"`
	ProjectID  string  `json:"project_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	Scopes     string  `json:"scopes"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
	Token      string  `json:"token,omitempty"`
	Hash       string  `json:"-"`
}

type Backup struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name,omitempty"`
	Name        string `json:"name"`
	Filename    string `json:"filename"`
	Kind        string `json:"kind"`
	Source      string `json:"source"`
	Note        string `json:"note"`
	Tags        string `json:"tags"`
	SizeBytes   int64  `json:"size_bytes"`
	Checksum    string `json:"checksum_sha256"`
	ContentType string `json:"content_type"`
	CreatedAt   string `json:"created_at"`
}

type Webhook struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	URL       string `json:"url"`
	Secret    string `json:"secret"`
	Events    string `json:"events"`
	CreatedAt string `json:"created_at"`
}

type Schedule struct {
	ID            string `json:"id"`
	ProjectID     string `json:"project_id"`
	Name          string `json:"name"`
	IntervalHours int    `json:"interval_hours"`
	CreatedAt     string `json:"created_at"`
	LastBackupAt  string `json:"last_backup_at,omitempty"`
	Overdue       bool   `json:"overdue"`
}

type Audit struct {
	ID        string `json:"id"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Meta      string `json:"meta"`
	CreatedAt string `json:"created_at"`
}

type BackupFilter struct {
	ProjectID string
	Query     string
	Limit     int
	Offset    int
}

type Stats struct {
	Projects         int      `json:"projects"`
	Backups          int      `json:"backups"`
	StorageBytes     int64    `json:"storage_bytes"`
	BackupsLast24h   int      `json:"backups_last_24h"`
	OverdueSchedules int      `json:"overdue_schedules"`
	RecentBackups    []Backup `json:"recent_backups"`
	RecentActivity   []Audit  `json:"recent_activity"`
}

func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	blobRoot := filepath.Join(dataDir, "blobs")
	if err := os.MkdirAll(blobRoot, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "tmp"), 0o755); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", filepath.Join(dataDir, "harbor.db"))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	db.SetConnMaxLifetime(0)
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db, blobs: blobRoot}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) BlobDir() string { return s.blobs }

func (s *Store) TmpDir() string { return filepath.Join(filepath.Dir(s.blobs), "tmp") }

func (s *Store) BlobPath(sum string) (string, error) {
	if len(sum) != 64 {
		return "", fmt.Errorf("invalid checksum")
	}
	for _, r := range sum {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return "", fmt.Errorf("invalid checksum")
		}
	}
	return filepath.Join(s.blobs, sum[:2], sum), nil
}

func Now() string { return time.Now().UTC().Format(time.RFC3339) }

func (s *Store) UserCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) CreateUser(ctx context.Context, email, name, passwordHash, role string) (User, error) {
	u := User{
		ID: id.New("usr_"), Email: strings.TrimSpace(email), Name: strings.TrimSpace(name),
		Role: role, CreatedAt: Now(), UpdatedAt: Now(),
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO users (id, email, name, password_hash, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Email, u.Name, passwordHash, u.Role, u.CreatedAt, u.UpdatedAt)
	u.Password = ""
	return u, err
}

func (s *Store) CreateOwner(ctx context.Context, email, name, passwordHash string) (User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE setup_guard SET id = 1 WHERE id = 1`); err != nil {
		return User{}, err
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return User{}, err
	}
	if n > 0 {
		return User{}, ErrConflict
	}
	u := User{
		ID: id.New("usr_"), Email: strings.TrimSpace(email), Name: strings.TrimSpace(name),
		Role: "owner", CreatedAt: Now(), UpdatedAt: Now(),
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO users (id, email, name, password_hash, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Email, u.Name, passwordHash, u.Role, u.CreatedAt, u.UpdatedAt); err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return u, nil
}

func (s *Store) scanUser(row interface{ Scan(dest ...any) error }) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Password, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, email, name, password_hash, role, created_at, updated_at FROM users WHERE email = ?`, strings.TrimSpace(email))
	u, err := s.scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) UserByID(ctx context.Context, userID string) (User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, email, name, password_hash, role, created_at, updated_at FROM users WHERE id = ?`, userID)
	u, err := s.scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, email, name, password_hash, role, created_at, updated_at FROM users ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := s.scanUser(rows)
		if err != nil {
			return nil, err
		}
		u.Password = ""
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, passwordHash, Now(), userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, userID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateSession(ctx context.Context, userID string, ttl time.Duration) (raw string, expires time.Time, err error) {
	raw, hash, _, err := id.Token("ses_")
	if err != nil {
		return "", time.Time{}, err
	}
	expires = time.Now().UTC().Add(ttl)
	_, err = s.db.ExecContext(ctx, `INSERT INTO sessions (id, user_id, token_hash, expires_at, created_at) VALUES (?, ?, ?, ?, ?)`,
		id.New("ses_"), userID, hash, expires.Format(time.RFC3339), Now())
	return raw, expires, err
}

func (s *Store) UserBySession(ctx context.Context, raw string) (User, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.email, u.name, u.password_hash, u.role, u.created_at, u.updated_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ?`, id.HashToken(raw), Now())
	u, err := s.scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	u.Password = ""
	return u, err
}

func (s *Store) DeleteSession(ctx context.Context, raw string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, id.HashToken(raw))
	return err
}

func (s *Store) CreateProject(ctx context.Context, name, description string, retentionCount, retentionDays int, quota int64) (Project, error) {
	base := id.Slug(name)
	slug := id.UniqueSlug(base, func(candidate string) bool {
		var n int
		_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects WHERE slug = ?`, candidate).Scan(&n)
		return n > 0
	})
	p := Project{
		ID: id.New("prj_"), Name: strings.TrimSpace(name), Slug: slug, Description: strings.TrimSpace(description),
		Status: "active", RetentionCount: retentionCount, RetentionDays: retentionDays, QuotaBytes: quota,
		CreatedAt: Now(), UpdatedAt: Now(),
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO projects (id, name, slug, description, status, retention_count, retention_days, quota_bytes, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Slug, p.Description, p.Status, p.RetentionCount, p.RetentionDays, p.QuotaBytes, p.CreatedAt, p.UpdatedAt)
	return p, err
}

const distinctUsageSQL = `SELECT COALESCE(SUM(size_bytes), 0) FROM (
  SELECT MAX(size_bytes) AS size_bytes FROM backups WHERE project_id = ? GROUP BY checksum_sha256
)`

func (s *Store) decorateProject(ctx context.Context, p *Project) error {
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM backups WHERE project_id = ?`, p.ID).Scan(&p.BackupCount); err != nil {
		return err
	}
	return s.db.QueryRowContext(ctx, distinctUsageSQL, p.ID).Scan(&p.UsedBytes)
}

func (s *Store) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, slug, description, status, retention_count, retention_days, quota_bytes, created_at, updated_at FROM projects ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.Description, &p.Status, &p.RetentionCount, &p.RetentionDays, &p.QuotaBytes, &p.CreatedAt, &p.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range out {
		if err := s.decorateProject(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) GetProject(ctx context.Context, projectID string) (Project, error) {
	var p Project
	err := s.db.QueryRowContext(ctx, `SELECT id, name, slug, description, status, retention_count, retention_days, quota_bytes, created_at, updated_at FROM projects WHERE id = ?`, projectID).
		Scan(&p.ID, &p.Name, &p.Slug, &p.Description, &p.Status, &p.RetentionCount, &p.RetentionDays, &p.QuotaBytes, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	if err != nil {
		return Project{}, err
	}
	if err := s.decorateProject(ctx, &p); err != nil {
		return Project{}, err
	}
	return p, nil
}

func (s *Store) UpdateProject(ctx context.Context, p Project) (Project, error) {
	p.UpdatedAt = Now()
	res, err := s.db.ExecContext(ctx, `UPDATE projects SET name = ?, description = ?, status = ?, retention_count = ?, retention_days = ?, quota_bytes = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(p.Name), strings.TrimSpace(p.Description), p.Status, p.RetentionCount, p.RetentionDays, p.QuotaBytes, p.UpdatedAt, p.ID)
	if err != nil {
		return Project{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Project{}, ErrNotFound
	}
	return s.GetProject(ctx, p.ID)
}

func (s *Store) DeleteProject(ctx context.Context, projectID string) error {
	sums, err := s.checksums(ctx, projectID)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, projectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	for sum := range sums {
		_ = s.releaseBlob(ctx, sum)
	}
	return nil
}

func (s *Store) checksums(ctx context.Context, projectID string) (map[string]struct{}, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT checksum_sha256 FROM backups WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var sum string
		if err := rows.Scan(&sum); err != nil {
			return nil, err
		}
		out[sum] = struct{}{}
	}
	return out, rows.Err()
}

func (s *Store) CreateAPIKey(ctx context.Context, projectID, name, scopes string) (APIKey, error) {
	raw, hash, prefix, err := id.Token("hb_")
	if err != nil {
		return APIKey{}, err
	}
	k := APIKey{
		ID: id.New("key_"), ProjectID: projectID, Name: strings.TrimSpace(name), Prefix: prefix,
		Scopes: scopes, CreatedAt: Now(), Token: raw,
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO api_keys (id, project_id, name, prefix, key_hash, scopes, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		k.ID, k.ProjectID, k.Name, k.Prefix, hash, k.Scopes, k.CreatedAt)
	return k, err
}

func (s *Store) ListAPIKeys(ctx context.Context, projectID string) ([]APIKey, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, project_id, name, prefix, scopes, last_used_at, created_at FROM api_keys WHERE project_id = ? ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKey
	for rows.Next() {
		var k APIKey
		var last sql.NullString
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.Name, &k.Prefix, &k.Scopes, &last, &k.CreatedAt); err != nil {
			return nil, err
		}
		if last.Valid {
			k.LastUsedAt = &last.String
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) DeleteAPIKey(ctx context.Context, projectID, keyID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM api_keys WHERE id = ? AND project_id = ?`, keyID, projectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) APIKeyByToken(ctx context.Context, raw string) (APIKey, error) {
	var k APIKey
	var last sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id, project_id, name, prefix, key_hash, scopes, last_used_at, created_at FROM api_keys WHERE key_hash = ?`, id.HashToken(raw)).
		Scan(&k.ID, &k.ProjectID, &k.Name, &k.Prefix, &k.Hash, &k.Scopes, &last, &k.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return APIKey{}, ErrNotFound
	}
	if last.Valid {
		k.LastUsedAt = &last.String
	}
	return k, err
}

func (s *Store) TouchAPIKey(ctx context.Context, keyID string) {
	_, _ = s.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = ? WHERE id = ?`, Now(), keyID)
}

func (s *Store) Usage(ctx context.Context, projectID string) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, distinctUsageSQL, projectID).Scan(&n)
	return n, err
}

func (s *Store) CreateBackup(ctx context.Context, b Backup) (Backup, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Backup{}, err
	}
	defer tx.Rollback()

	now := Now()
	res, err := tx.ExecContext(ctx, `UPDATE projects SET updated_at = ? WHERE id = ?`, now, b.ProjectID)
	if err != nil {
		return Backup{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Backup{}, ErrNotFound
	}
	var status string
	var quota int64
	if err := tx.QueryRowContext(ctx, `SELECT status, quota_bytes FROM projects WHERE id = ?`, b.ProjectID).Scan(&status, &quota); err != nil {
		return Backup{}, err
	}
	if status != "active" {
		return Backup{}, ErrPaused
	}
	if quota > 0 {
		var used, existing int64
		if err := tx.QueryRowContext(ctx, distinctUsageSQL, b.ProjectID).Scan(&used); err != nil {
			return Backup{}, err
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM backups WHERE project_id = ? AND checksum_sha256 = ?`, b.ProjectID, b.Checksum).Scan(&existing); err != nil {
			return Backup{}, err
		}
		extra := b.SizeBytes
		if existing > 0 {
			extra = 0
		}
		if used+extra > quota {
			return Backup{}, ErrQuota
		}
	}
	b.ID = id.New("bak_")
	b.CreatedAt = now
	if _, err := tx.ExecContext(ctx, `INSERT INTO backups (id, project_id, name, filename, kind, source, note, tags, size_bytes, checksum_sha256, content_type, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ID, b.ProjectID, b.Name, b.Filename, b.Kind, b.Source, b.Note, b.Tags, b.SizeBytes, b.Checksum, b.ContentType, b.CreatedAt); err != nil {
		return Backup{}, err
	}
	if err := tx.Commit(); err != nil {
		return Backup{}, err
	}
	return b, nil
}

func (s *Store) ReleaseUnused(ctx context.Context, sum string) {
	_ = s.releaseBlob(ctx, sum)
}

func (s *Store) ListBackups(ctx context.Context, f BackupFilter) ([]Backup, int, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	q := strings.TrimSpace(f.Query)
	like := "%" + escapeLike(q) + "%"
	where := `WHERE (? = '' OR b.project_id = ?) AND (? = '' OR b.name LIKE ? ESCAPE '\' OR b.filename LIKE ? ESCAPE '\' OR b.source LIKE ? ESCAPE '\' OR b.tags LIKE ? ESCAPE '\' OR b.checksum_sha256 LIKE ? ESCAPE '\')`
	args := []any{f.ProjectID, f.ProjectID, q, like, like, like, like, like}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM backups b `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT b.id, b.project_id, p.name, b.name, b.filename, b.kind, b.source, b.note, b.tags, b.size_bytes, b.checksum_sha256, b.content_type, b.created_at
		FROM backups b JOIN projects p ON p.id = b.project_id `+where+` ORDER BY b.created_at DESC, b.rowid DESC LIMIT ? OFFSET ?`, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Backup
	for rows.Next() {
		b, err := scanBackup(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, b)
	}
	return out, total, rows.Err()
}

func scanBackup(row interface{ Scan(dest ...any) error }) (Backup, error) {
	var b Backup
	err := row.Scan(&b.ID, &b.ProjectID, &b.ProjectName, &b.Name, &b.Filename, &b.Kind, &b.Source, &b.Note, &b.Tags, &b.SizeBytes, &b.Checksum, &b.ContentType, &b.CreatedAt)
	return b, err
}

func (s *Store) GetBackup(ctx context.Context, backupID string) (Backup, error) {
	row := s.db.QueryRowContext(ctx, `SELECT b.id, b.project_id, p.name, b.name, b.filename, b.kind, b.source, b.note, b.tags, b.size_bytes, b.checksum_sha256, b.content_type, b.created_at
		FROM backups b JOIN projects p ON p.id = b.project_id WHERE b.id = ?`, backupID)
	b, err := scanBackup(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Backup{}, ErrNotFound
	}
	return b, err
}

func (s *Store) DeleteBackup(ctx context.Context, backupID string) (Backup, error) {
	b, err := s.GetBackup(ctx, backupID)
	if err != nil {
		return Backup{}, err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM backups WHERE id = ?`, backupID); err != nil {
		return Backup{}, err
	}
	_ = s.releaseBlob(ctx, b.Checksum)
	return b, nil
}

func (s *Store) releaseBlob(ctx context.Context, sum string) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM backups WHERE checksum_sha256 = ?`, sum).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	path, err := s.BlobPath(sum)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_ = os.Remove(filepath.Dir(path))
	return nil
}

func (s *Store) EnforceRetention(ctx context.Context, projectID string) ([]Backup, error) {
	p, err := s.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	var doomed []string
	if p.RetentionDays > 0 {
		cutoff := time.Now().UTC().Add(-time.Duration(p.RetentionDays) * 24 * time.Hour).Format(time.RFC3339)
		rows, err := s.db.QueryContext(ctx, `SELECT id FROM backups WHERE project_id = ? AND created_at < ?`, projectID, cutoff)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var bid string
			if err := rows.Scan(&bid); err != nil {
				rows.Close()
				return nil, err
			}
			doomed = append(doomed, bid)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	if p.RetentionCount > 0 {
		rows, err := s.db.QueryContext(ctx, `SELECT id FROM backups WHERE project_id = ? ORDER BY created_at DESC, rowid DESC LIMIT -1 OFFSET ?`, projectID, p.RetentionCount)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var bid string
			if err := rows.Scan(&bid); err != nil {
				rows.Close()
				return nil, err
			}
			doomed = append(doomed, bid)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	seen := map[string]struct{}{}
	var removed []Backup
	for _, bid := range doomed {
		if _, ok := seen[bid]; ok {
			continue
		}
		seen[bid] = struct{}{}
		b, err := s.DeleteBackup(ctx, bid)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return removed, err
		}
		removed = append(removed, b)
	}
	return removed, nil
}

func (s *Store) EnforceAllRetention(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM projects`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var pid string
		if err := rows.Scan(&pid); err != nil {
			return err
		}
		ids = append(ids, pid)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, pid := range ids {
		if _, err := s.EnforceRetention(ctx, pid); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GCOrphanBlobs(ctx context.Context) error {
	return filepath.WalkDir(s.blobs, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		sum := d.Name()
		var n int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM backups WHERE checksum_sha256 = ?`, sum).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			_ = os.Remove(path)
		}
		return nil
	})
}

func (s *Store) CreateWebhook(ctx context.Context, projectID, url, events string) (Webhook, error) {
	secret, err := id.Secret()
	if err != nil {
		return Webhook{}, err
	}
	w := Webhook{ID: id.New("wh_"), ProjectID: projectID, URL: strings.TrimSpace(url), Secret: secret, Events: events, CreatedAt: Now()}
	_, err = s.db.ExecContext(ctx, `INSERT INTO webhooks (id, project_id, url, secret, events, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		w.ID, w.ProjectID, w.URL, w.Secret, w.Events, w.CreatedAt)
	return w, err
}

func (s *Store) ListWebhooks(ctx context.Context, projectID string) ([]Webhook, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, project_id, url, secret, events, created_at FROM webhooks WHERE project_id = ? ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Webhook
	for rows.Next() {
		var w Webhook
		if err := rows.Scan(&w.ID, &w.ProjectID, &w.URL, &w.Secret, &w.Events, &w.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) DeleteWebhook(ctx context.Context, projectID, webhookID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM webhooks WHERE id = ? AND project_id = ?`, webhookID, projectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateSchedule(ctx context.Context, projectID, name string, hours int) (Schedule, error) {
	sc := Schedule{ID: id.New("sch_"), ProjectID: projectID, Name: strings.TrimSpace(name), IntervalHours: hours, CreatedAt: Now()}
	_, err := s.db.ExecContext(ctx, `INSERT INTO schedules (id, project_id, name, interval_hours, created_at) VALUES (?, ?, ?, ?, ?)`,
		sc.ID, sc.ProjectID, sc.Name, sc.IntervalHours, sc.CreatedAt)
	if err != nil {
		return Schedule{}, err
	}
	return s.decorateSchedule(ctx, sc), nil
}

func (s *Store) decorateSchedule(ctx context.Context, sc Schedule) Schedule {
	var last sql.NullString
	_ = s.db.QueryRowContext(ctx, `SELECT MAX(created_at) FROM backups WHERE project_id = ?`, sc.ProjectID).Scan(&last)
	anchor := sc.CreatedAt
	if last.Valid && last.String != "" {
		sc.LastBackupAt = last.String
		anchor = last.String
	}
	if t, err := time.Parse(time.RFC3339, anchor); err == nil {
		sc.Overdue = time.Since(t) > time.Duration(sc.IntervalHours)*time.Hour
	}
	return sc
}

func (s *Store) ListSchedules(ctx context.Context, projectID string) ([]Schedule, error) {
	q := `SELECT id, project_id, name, interval_hours, created_at FROM schedules`
	args := []any{}
	if projectID != "" {
		q += ` WHERE project_id = ?`
		args = append(args, projectID)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	var out []Schedule
	for rows.Next() {
		var sc Schedule
		if err := rows.Scan(&sc.ID, &sc.ProjectID, &sc.Name, &sc.IntervalHours, &sc.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, sc)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i] = s.decorateSchedule(ctx, out[i])
	}
	return out, nil
}

func (s *Store) DeleteSchedule(ctx context.Context, projectID, scheduleID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM schedules WHERE id = ? AND project_id = ?`, scheduleID, projectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AddAudit(ctx context.Context, actor, action, target, meta string) {
	_, _ = s.db.ExecContext(ctx, `INSERT INTO audit_logs (id, actor, action, target, meta, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id.New("aud_"), actor, action, target, meta, Now())
}

func (s *Store) ListAudit(ctx context.Context, limit, offset int) ([]Audit, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, actor, action, target, meta, created_at FROM audit_logs ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Audit
	for rows.Next() {
		var a Audit
		if err := rows.Scan(&a.ID, &a.Actor, &a.Action, &a.Target, &a.Meta, &a.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}

func (s *Store) GetSetting(ctx context.Context, key, fallback string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback, nil
	}
	return v, err
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *Store) Stats(ctx context.Context) (Stats, error) {
	var st Stats
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects`).Scan(&st.Projects); err != nil {
		return st, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM backups`).Scan(&st.Backups); err != nil {
		return st, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(size_bytes), 0) FROM (SELECT MAX(size_bytes) AS size_bytes FROM backups GROUP BY checksum_sha256)`).Scan(&st.StorageBytes); err != nil {
		return st, err
	}
	since := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM backups WHERE created_at >= ?`, since).Scan(&st.BackupsLast24h); err != nil {
		return st, err
	}
	schedules, err := s.ListSchedules(ctx, "")
	if err != nil {
		return st, err
	}
	for _, sc := range schedules {
		if sc.Overdue {
			st.OverdueSchedules++
		}
	}
	recent, _, err := s.ListBackups(ctx, BackupFilter{Limit: 8})
	if err != nil {
		return st, err
	}
	st.RecentBackups = recent
	activity, _, err := s.ListAudit(ctx, 8, 0)
	if err != nil {
		return st, err
	}
	st.RecentActivity = activity
	if st.RecentBackups == nil {
		st.RecentBackups = []Backup{}
	}
	if st.RecentActivity == nil {
		st.RecentActivity = []Audit{}
	}
	return st, nil
}

func (s *Store) LatestBackupAt(ctx context.Context, projectID string) (string, error) {
	var last sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT MAX(created_at) FROM backups WHERE project_id = ?`, projectID).Scan(&last)
	if err != nil || !last.Valid {
		return "", err
	}
	return last.String, nil
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
