package postgres

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var migrationsFS embed.FS

type Migrator struct {
	pool *pgxpool.Pool
}

func NewMigrator(pool *pgxpool.Pool) *Migrator {
	return &Migrator{pool: pool}
}

func (m *Migrator) Run(ctx context.Context) error {
	
	if err := m.createMigrationsTable(ctx); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	migrations, err := m.getMigrationFiles()
	if err != nil {
		return fmt.Errorf("failed to get migration files: %w", err)
	}

	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	for _, migration := range migrations {
		if _, ok := applied[migration]; ok {
			continue
		}

		if err := m.applyMigration(ctx, migration); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration, err)
		}

		fmt.Printf("Applied migration: %s\n", migration)
	}

	return nil
}

func (m *Migrator) createMigrationsTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`
	_, err := m.pool.Exec(ctx, query)
	return err
}

func (m *Migrator) getMigrationFiles() ([]string, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}

	var migrations []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			migrations = append(migrations, entry.Name())
		}
	}

	sort.Strings(migrations)
	return migrations, nil
}

func (m *Migrator) getAppliedMigrations(ctx context.Context) (map[string]bool, error) {
	query := `SELECT version FROM schema_migrations`
	rows, err := m.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}

	return applied, nil
}

func (m *Migrator) applyMigration(ctx context.Context, filename string) error {
	
	content, err := migrationsFS.ReadFile("migrations/" + filename)
	if err != nil {
		return fmt.Errorf("failed to read migration file: %w", err)
	}

	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, string(content)); err != nil {
		return fmt.Errorf("failed to execute migration: %w", err)
	}

	recordQuery := `INSERT INTO schema_migrations (version) VALUES ($1)`
	if _, err := tx.Exec(ctx, recordQuery, filename); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}

	return nil
}

func (m *Migrator) Rollback(ctx context.Context, n int) error {
	
	return fmt.Errorf("rollback not implemented - requires down migration files")
}

func (m *Migrator) Reset(ctx context.Context) error {
	
	dropQuery := `
		DROP SCHEMA public CASCADE;
		CREATE SCHEMA public;
		GRANT ALL ON SCHEMA public TO postgres;
		GRANT ALL ON SCHEMA public TO public;
	`
	
	if _, err := m.pool.Exec(ctx, dropQuery); err != nil {
		return fmt.Errorf("failed to reset database: %w", err)
	}

	return m.Run(ctx)
}