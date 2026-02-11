package database

import (
	"context"
	"time"

	"github.com/uptrace/bun"
)

// AdditionalRequest tracks the state of each additional user request within a ticket.
// The request content lives in the ticket file; this table tracks processing status.
type AdditionalRequest struct {
	bun.BaseModel `bun:"table:additional_requests,alias:ar"`

	ID                   int64     `bun:"id,pk,autoincrement" json:"id"`
	TicketPath           string    `bun:"ticket_path,notnull" json:"ticket_path"` // absolute path to parent ticket
	RequestNum           int       `bun:"request_num,notnull" json:"request_num"` // 1-based request number
	Status               string    `bun:"status,notnull" json:"status"`           // "created", "in_progress", "completed", "completed + verified"
	IsDraft              bool      `bun:"is_draft,notnull,default:false" json:"is_draft"` // draft requests are not actionable until activated
	Content              string    `bun:"content,default:''" json:"content"`             // request content (stored for deferred file writing)
	OriginalTicketStatus string    `bun:"original_ticket_status,default:''" json:"original_ticket_status"` // ticket's frontmatter status when this request was created
	CreatedAt            time.Time `bun:"created_at,notnull" json:"created_at"`
}

// CreateAdditionalRequest inserts a new additional request record.
// originalStatus is the ticket's frontmatter status at creation time, used to
// restore immutable history after the worker processes the additional request.
func CreateAdditionalRequest(ctx context.Context, ticketPath string, requestNum int, isDraft bool, content string, originalStatus string) (int64, error) {
	ar := &AdditionalRequest{
		TicketPath:           ticketPath,
		RequestNum:           requestNum,
		Status:               "created",
		IsDraft:              isDraft,
		Content:              content,
		OriginalTicketStatus: originalStatus,
		CreatedAt:            time.Now(),
	}
	_, err := DB.NewInsert().Model(ar).Exec(ctx)
	if err != nil {
		return 0, err
	}
	return ar.ID, nil
}

// GetAdditionalRequests returns all additional requests for a ticket, ordered by request_num.
func GetAdditionalRequests(ctx context.Context, ticketPath string) ([]AdditionalRequest, error) {
	var reqs []AdditionalRequest
	err := DB.NewSelect().
		Model(&reqs).
		Where("ticket_path = ?", ticketPath).
		OrderExpr("request_num ASC").
		Scan(ctx)
	return reqs, err
}

// GetAdditionalRequest returns a single additional request by ticket path and request number.
func GetAdditionalRequest(ctx context.Context, ticketPath string, requestNum int) (*AdditionalRequest, error) {
	ar := new(AdditionalRequest)
	err := DB.NewSelect().
		Model(ar).
		Where("ticket_path = ?", ticketPath).
		Where("request_num = ?", requestNum).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return ar, nil
}

// UpdateAdditionalRequestStatus updates the status of a specific additional request.
func UpdateAdditionalRequestStatus(ctx context.Context, ticketPath string, requestNum int, status string) error {
	_, err := DB.NewUpdate().
		Model((*AdditionalRequest)(nil)).
		Set("status = ?", status).
		Where("ticket_path = ?", ticketPath).
		Where("request_num = ?", requestNum).
		Exec(ctx)
	return err
}

// ActivateAdditionalRequest sets is_draft to false, making the request actionable.
func ActivateAdditionalRequest(ctx context.Context, ticketPath string, requestNum int) error {
	_, err := DB.NewUpdate().
		Model((*AdditionalRequest)(nil)).
		Set("is_draft = ?", false).
		Where("ticket_path = ?", ticketPath).
		Where("request_num = ?", requestNum).
		Exec(ctx)
	return err
}

// CountAdditionalRequests returns how many additional requests exist for a ticket.
func CountAdditionalRequests(ctx context.Context, ticketPath string) (int, error) {
	count, err := DB.NewSelect().
		Model((*AdditionalRequest)(nil)).
		Where("ticket_path = ?", ticketPath).
		Count(ctx)
	return count, err
}

// MaxRequestNum returns the highest request_num for a ticket, or 0 if none exist.
func MaxRequestNum(ctx context.Context, ticketPath string) (int, error) {
	var maxNum int
	err := DB.NewSelect().
		Model((*AdditionalRequest)(nil)).
		ColumnExpr("COALESCE(MAX(request_num), 0)").
		Where("ticket_path = ?", ticketPath).
		Scan(ctx, &maxNum)
	return maxNum, err
}

// GetOriginalTicketStatus returns the saved original frontmatter status for a ticket
// from any of its additional requests. Returns the status from the first request that
// has a non-empty original_ticket_status. Returns "" if none found.
func GetOriginalTicketStatus(ctx context.Context, ticketPath string) (string, error) {
	ar := new(AdditionalRequest)
	err := DB.NewSelect().
		Model(ar).
		Column("original_ticket_status").
		Where("ticket_path = ?", ticketPath).
		Where("original_ticket_status != ''").
		OrderExpr("request_num ASC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return "", err
	}
	return ar.OriginalTicketStatus, nil
}

// GetAdditionalRequestContent returns the stored content for a specific additional request.
func GetAdditionalRequestContent(ctx context.Context, ticketPath string, requestNum int) (string, error) {
	ar := new(AdditionalRequest)
	err := DB.NewSelect().
		Model(ar).
		Column("content").
		Where("ticket_path = ?", ticketPath).
		Where("request_num = ?", requestNum).
		Scan(ctx)
	if err != nil {
		return "", err
	}
	return ar.Content, nil
}
