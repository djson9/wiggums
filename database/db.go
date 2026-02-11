package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

var DB *bun.DB

func Init() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	dbDir := filepath.Join(home, ".wiggums")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return err
	}

	dbPath := filepath.Join(dbDir, "wiggums.db")
	dsn := "file:" + dbPath + "?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL"
	sqldb, err := sql.Open(sqliteshim.ShimName, dsn)
	if err != nil {
		return err
	}

	sqldb.SetMaxOpenConns(1)

	DB = bun.NewDB(sqldb, sqlitedialect.New())
	return nil
}

func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}

func Migrate(ctx context.Context) error {
	_, err := DB.NewCreateTable().
		Model((*TicketRun)(nil)).
		IfNotExists().
		Exec(ctx)
	if err != nil {
		return err
	}
	_, err = DB.NewCreateTable().
		Model((*AdditionalRequest)(nil)).
		IfNotExists().
		Exec(ctx)
	if err != nil {
		return err
	}

	// Add is_draft column if it doesn't exist (for databases created before draft feature).
	// SQLite will error if column already exists, so we ignore the error.
	_, _ = DB.ExecContext(ctx, `ALTER TABLE additional_requests ADD COLUMN is_draft BOOLEAN NOT NULL DEFAULT false`)

	// Add content column if it doesn't exist (for databases created before runtime draft feature).
	// Stores draft content so it can be written to the ticket file at runtime instead of creation time.
	_, _ = DB.ExecContext(ctx, `ALTER TABLE additional_requests ADD COLUMN content TEXT DEFAULT ''`)

	// Add ticket_created_at column if it doesn't exist (for databases created before this feature).
	_, _ = DB.ExecContext(ctx, `ALTER TABLE ticket_runs ADD COLUMN ticket_created_at TIMESTAMP`)

	// Add original_ticket_status column if it doesn't exist. Persists the ticket's
	// frontmatter status at the time the additional request was created, so the
	// original status can be restored after processing (history immutability).
	_, _ = DB.ExecContext(ctx, `ALTER TABLE additional_requests ADD COLUMN original_ticket_status TEXT DEFAULT ''`)

	return nil
}
