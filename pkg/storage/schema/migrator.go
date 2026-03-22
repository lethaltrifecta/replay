package schema

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var migrationNames = []string{
	"001_initial_schema.sql",
	"002_baselines_and_drift.sql",
	"003_drift_unique_constraint.sql",
}

// Migrate applies the shared replay schema to the provided PostgreSQL pool.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	for _, name := range migrationNames {
		sql, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("execute migration %s: %w", name, err)
		}
	}
	return nil
}
