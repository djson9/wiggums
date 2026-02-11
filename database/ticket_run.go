package database

import (
	"context"
	"time"

	"github.com/uptrace/bun"
)

// TicketRun tracks each individual processing cycle of a ticket.
type TicketRun struct {
	bun.BaseModel `bun:"table:ticket_runs,alias:tr"`

	ID              int64      `bun:"id,pk,autoincrement" json:"id"`
	Workspace       string     `bun:"workspace,notnull" json:"workspace"`
	Ticket          string     `bun:"ticket,notnull" json:"ticket"`       // filename e.g. "1771436012_AGSUP-2809_MD_SUI.md"
	Phase           string     `bun:"phase,notnull" json:"phase"`         // "working" or "verifying"
	StartedAt       time.Time  `bun:"started_at,notnull" json:"started_at"`
	CompletedAt     *time.Time `bun:"completed_at" json:"completed_at,omitempty"` // nil while in progress
	ExitCode        *int       `bun:"exit_code" json:"exit_code,omitempty"`
	TicketCreatedAt *time.Time `bun:"ticket_created_at" json:"ticket_created_at,omitempty"` // ticket file creation time (from filename epoch)
}

// StartRun creates a new ticket run record and returns its ID.
// ticketCreatedAt is optional — pass nil if unknown.
func StartRun(ctx context.Context, workspace, ticket, phase string, ticketCreatedAt *time.Time) (int64, error) {
	run := &TicketRun{
		Workspace:       workspace,
		Ticket:          ticket,
		Phase:           phase,
		StartedAt:       time.Now(),
		TicketCreatedAt: ticketCreatedAt,
	}
	_, err := DB.NewInsert().Model(run).Exec(ctx)
	if err != nil {
		return 0, err
	}
	return run.ID, nil
}

// CompleteRun marks a ticket run as completed.
func CompleteRun(ctx context.Context, id int64, exitCode int) error {
	now := time.Now()
	_, err := DB.NewUpdate().
		Model((*TicketRun)(nil)).
		Set("completed_at = ?", now).
		Set("exit_code = ?", exitCode).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

// CompleteOpenRuns marks all open runs for a workspace as completed (e.g., on shutdown).
func CompleteOpenRuns(ctx context.Context, workspace string, exitCode int) error {
	now := time.Now()
	_, err := DB.NewUpdate().
		Model((*TicketRun)(nil)).
		Set("completed_at = ?", now).
		Set("exit_code = ?", exitCode).
		Where("workspace = ?", workspace).
		Where("completed_at IS NULL").
		Exec(ctx)
	return err
}

// RecentRuns returns the most recent runs for a workspace, ordered by started_at desc.
func RecentRuns(ctx context.Context, workspace string, limit int) ([]TicketRun, error) {
	var runs []TicketRun
	err := DB.NewSelect().
		Model(&runs).
		Where("workspace = ?", workspace).
		OrderExpr("started_at DESC").
		Limit(limit).
		Scan(ctx)
	return runs, err
}

// ActiveRuns returns all in-progress runs (completed_at IS NULL) for a workspace.
func ActiveRuns(ctx context.Context, workspace string) ([]TicketRun, error) {
	var runs []TicketRun
	err := DB.NewSelect().
		Model(&runs).
		Where("workspace = ?", workspace).
		Where("completed_at IS NULL").
		OrderExpr("started_at ASC").
		Scan(ctx)
	return runs, err
}

// RecentCompletedRuns returns completed runs for a workspace within a duration.
func RecentCompletedRuns(ctx context.Context, workspace string, since time.Time) ([]TicketRun, error) {
	var runs []TicketRun
	err := DB.NewSelect().
		Model(&runs).
		Where("workspace = ?", workspace).
		Where("completed_at IS NOT NULL").
		Where("completed_at > ?", since).
		OrderExpr("completed_at DESC").
		Scan(ctx)
	return runs, err
}

// AllTicketDurations returns total run duration per ticket across all workspaces.
// Key format is "workspace:ticket" (e.g. "myws:1234_title.md").
func AllTicketDurations(ctx context.Context) (map[string]time.Duration, error) {
	var runs []TicketRun
	err := DB.NewSelect().
		Model(&runs).
		Where("completed_at IS NOT NULL").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string]time.Duration)
	for _, r := range runs {
		if r.CompletedAt == nil {
			continue
		}
		key := r.Workspace + ":" + r.Ticket
		m[key] += r.CompletedAt.Sub(r.StartedAt)
	}
	return m, nil
}

// HasRecentRuns returns true if any run for the workspace completed after the cutoff.
func HasRecentRuns(ctx context.Context, workspace string, cutoff time.Time) (bool, error) {
	count, err := DB.NewSelect().
		Model((*TicketRun)(nil)).
		Where("workspace = ?", workspace).
		Where("completed_at > ? OR completed_at IS NULL", cutoff).
		Count(ctx)
	return count > 0, err
}
