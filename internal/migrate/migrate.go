// Package migrate provides database migration support with transaction safety,
// advisory locking, dirty-state tracking, and up/down/status operations.
package migrate

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Advisory lock ID — prevents concurrent migration runs across processes.
const advisoryLockID int64 = 7_264_613_482 // arbitrary unique number

// Migrator runs SQL migrations against a PostgreSQL database.
type Migrator struct {
	pool *pgxpool.Pool
	dir  string
}

// New creates a Migrator that reads .sql files from dir.
func New(pool *pgxpool.Pool, dir string) *Migrator {
	return &Migrator{pool: pool, dir: dir}
}

// Up applies all pending migrations in order.
func (m *Migrator) Up(ctx context.Context) error {
	if err := m.ensureTable(ctx); err != nil {
		return err
	}
	if err := m.checkDirty(ctx); err != nil {
		return err
	}

	conn, err := m.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	if err := m.lock(ctx, conn); err != nil {
		return err
	}
	defer m.unlock(ctx, conn)

	files, err := m.upFiles()
	if err != nil {
		return err
	}

	for _, f := range files {
		version := extractVersion(filepath.Base(f))
		applied, err := m.isApplied(ctx, version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		if err := m.applyUp(ctx, conn, f, version); err != nil {
			return err
		}
	}

	return nil
}

// Down rolls back the most recent migration.
func (m *Migrator) Down(ctx context.Context) error {
	if err := m.ensureTable(ctx); err != nil {
		return err
	}
	if err := m.checkDirty(ctx); err != nil {
		return err
	}

	conn, err := m.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	if err := m.lock(ctx, conn); err != nil {
		return err
	}
	defer m.unlock(ctx, conn)

	current, err := m.currentVersion(ctx)
	if err != nil {
		return err
	}
	if current == "" {
		slog.Info("no migrations to roll back")
		return nil
	}

	downFile := m.downFileFor(current)
	if downFile == "" {
		return fmt.Errorf("no down migration found for version %s", current)
	}

	return m.applyDown(ctx, conn, downFile, current)
}

// Status prints applied and pending migrations.
func (m *Migrator) Status(ctx context.Context) error {
	if err := m.ensureTable(ctx); err != nil {
		return err
	}

	files, err := m.upFiles()
	if err != nil {
		return err
	}

	rows, err := m.pool.Query(ctx,
		`SELECT version, applied_at, dirty FROM schema_migrations ORDER BY version`)
	if err != nil {
		return fmt.Errorf("query migrations: %w", err)
	}
	defer rows.Close()

	applied := map[string]struct {
		at    time.Time
		dirty bool
	}{}
	for rows.Next() {
		var v string
		var at time.Time
		var dirty bool
		if err := rows.Scan(&v, &at, &dirty); err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		applied[v] = struct {
			at    time.Time
			dirty bool
		}{at, dirty}
	}

	fmt.Printf("%-8s  %-8s  %-20s  %s\n", "VERSION", "STATUS", "APPLIED AT", "FILE")
	fmt.Println(strings.Repeat("-", 72))

	for _, f := range files {
		version := extractVersion(filepath.Base(f))
		if info, ok := applied[version]; ok {
			status := "applied"
			if info.dirty {
				status = "DIRTY"
			}
			fmt.Printf("%-8s  %-8s  %-20s  %s\n",
				version, status, info.at.Format("2006-01-02 15:04:05"), filepath.Base(f))
		} else {
			fmt.Printf("%-8s  %-8s  %-20s  %s\n",
				version, "pending", "", filepath.Base(f))
		}
	}

	return nil
}

// Reset forces a dirty migration to clean state so it can be re-applied.
// Use after manually fixing the database.
func (m *Migrator) Reset(ctx context.Context) error {
	if err := m.ensureTable(ctx); err != nil {
		return err
	}

	tag, err := m.pool.Exec(ctx,
		`DELETE FROM schema_migrations WHERE dirty = true`)
	if err != nil {
		return fmt.Errorf("reset dirty migrations: %w", err)
	}
	slog.Info("reset dirty migrations", "removed", tag.RowsAffected())
	return nil
}

// --- internal helpers ---

func (m *Migrator) ensureTable(ctx context.Context) error {
	_, err := m.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			dirty      BOOLEAN NOT NULL DEFAULT FALSE
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// Add dirty column if upgrading from old schema
	_, _ = m.pool.Exec(ctx, `ALTER TABLE schema_migrations ADD COLUMN IF NOT EXISTS dirty BOOLEAN NOT NULL DEFAULT FALSE`)

	return nil
}

func (m *Migrator) checkDirty(ctx context.Context) error {
	var version string
	err := m.pool.QueryRow(ctx,
		`SELECT version FROM schema_migrations WHERE dirty = true LIMIT 1`).Scan(&version)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check dirty: %w", err)
	}
	return fmt.Errorf("migration %s is dirty — fix the database manually then run 'migrate reset'", version)
}

func (m *Migrator) lock(ctx context.Context, conn *pgxpool.Conn) error {
	_, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockID)
	if err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}
	return nil
}

func (m *Migrator) unlock(ctx context.Context, conn *pgxpool.Conn) {
	_, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, advisoryLockID)
}

func (m *Migrator) upFiles() ([]string, error) {
	files, err := filepath.Glob(filepath.Join(m.dir, "*.up.sql"))
	if err != nil {
		return nil, fmt.Errorf("glob migrations: %w", err)
	}
	sort.Strings(files)
	return files, nil
}

func (m *Migrator) isApplied(ctx context.Context, version string) (bool, error) {
	var exists bool
	err := m.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check version %s: %w", version, err)
	}
	return exists, nil
}

func (m *Migrator) currentVersion(ctx context.Context) (string, error) {
	var version string
	err := m.pool.QueryRow(ctx,
		`SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&version)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("current version: %w", err)
	}
	return version, nil
}

// isAlterTypeAddValue checks if a SQL line is an ALTER TYPE ... ADD VALUE statement.
func isAlterTypeAddValue(line string) bool {
	upper := strings.ToUpper(strings.TrimSpace(line))
	return strings.HasPrefix(upper, "ALTER TYPE") && strings.Contains(upper, "ADD VALUE")
}

// hasAlterTypeAddValue checks if any line in the content is ALTER TYPE ... ADD VALUE.
func hasAlterTypeAddValue(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		if isAlterTypeAddValue(line) {
			return true
		}
	}
	return false
}

func (m *Migrator) applyUp(ctx context.Context, conn *pgxpool.Conn, file, version string) error {
	content, err := os.ReadFile(file) // #nosec G304 -- file comes from listing the migrations directory, not from user input
	if err != nil {
		return fmt.Errorf("read %s: %w", file, err)
	}

	slog.Info("applying migration", "version", version, "file", filepath.Base(file))

	// Mark as dirty before executing
	_, err = conn.Exec(ctx,
		`INSERT INTO schema_migrations (version, dirty) VALUES ($1, true)`, version)
	if err != nil {
		return fmt.Errorf("mark dirty %s: %w", version, err)
	}

	sql := string(content)

	if hasAlterTypeAddValue(sql) {
		// PostgreSQL requires ALTER TYPE ... ADD VALUE to run outside any transaction,
		// and the new values can't be used until they're committed. We split the file:
		// 1. Run each ALTER TYPE ... ADD VALUE individually (auto-committed)
		// 2. Run the remaining SQL as a batch
		var remaining []string
		for _, line := range strings.Split(sql, "\n") {
			if isAlterTypeAddValue(line) {
				if _, err := conn.Exec(ctx, line); err != nil {
					return fmt.Errorf("apply %s (enum): %w", version, err)
				}
			} else {
				remaining = append(remaining, line)
			}
		}
		rest := strings.Join(remaining, "\n")
		if strings.TrimSpace(rest) != "" {
			if _, err := conn.Exec(ctx, rest); err != nil {
				return fmt.Errorf("apply %s: %w", version, err)
			}
		}
	} else {
		// Execute inside a transaction
		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx %s: %w", version, err)
		}

		_, err = tx.Exec(ctx, sql)
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit %s: %w", version, err)
		}
	}

	// Mark as clean
	_, err = conn.Exec(ctx,
		`UPDATE schema_migrations SET dirty = false, applied_at = NOW() WHERE version = $1`, version)
	if err != nil {
		return fmt.Errorf("unmark dirty %s: %w", version, err)
	}

	slog.Info("migration applied", "version", version)
	return nil
}

func (m *Migrator) applyDown(ctx context.Context, conn *pgxpool.Conn, file, version string) error {
	content, err := os.ReadFile(file) // #nosec G304 -- file comes from listing the migrations directory, not from user input
	if err != nil {
		return fmt.Errorf("read %s: %w", file, err)
	}

	slog.Info("rolling back migration", "version", version, "file", filepath.Base(file))

	// Mark as dirty
	_, err = conn.Exec(ctx,
		`UPDATE schema_migrations SET dirty = true WHERE version = $1`, version)
	if err != nil {
		return fmt.Errorf("mark dirty %s: %w", version, err)
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx %s: %w", version, err)
	}

	_, err = tx.Exec(ctx, string(content))
	if err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("rollback %s: %w", version, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rollback %s: %w", version, err)
	}

	// Remove the version record
	_, err = conn.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, version)
	if err != nil {
		return fmt.Errorf("delete version %s: %w", version, err)
	}

	slog.Info("migration rolled back", "version", version)
	return nil
}

func (m *Migrator) downFileFor(version string) string {
	pattern := filepath.Join(m.dir, version+"_*.down.sql")
	matches, _ := filepath.Glob(pattern)
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

// extractVersion returns the numeric prefix: "000001_init.up.sql" → "000001"
func extractVersion(filename string) string {
	parts := strings.SplitN(filename, "_", 2)
	if len(parts) == 0 {
		return filename
	}
	// Validate it's numeric
	if _, err := strconv.Atoi(parts[0]); err != nil {
		return filename
	}
	return parts[0]
}
