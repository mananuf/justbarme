package activity

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/store"
	"github.com/mananuf/justbarme/internal/store/sqlc"
)

type Service struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func pgUUIDOrNil(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

func pgTextOrNil(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func pgTimestamptzOrNil(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func toEntry(r sqlc.ListActivityRow) Entry {
	return Entry{
		ID: r.ID, Type: r.Type, ActorID: r.ActorID,
		OccurredAt: r.OccurredAt.Time, Summary: r.Summary, AmountKobo: r.AmountKobo,
	}
}

// List returns the unified activity feed, newest first, filtered per f.
// See docs/PHASE_EXPENSES_DASHBOARD_ACTIVITY_REPORTS.md §3 for why this is
// a query-time UNION (internal/store/queries/activity.sql's ListActivity)
// rather than a dedicated writer table.
func (s *Service) List(ctx context.Context, userID, businessID uuid.UUID, f Filter, limit int32) ([]Entry, error) {
	var rows []sqlc.ListActivityRow
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		r, err := q.ListActivity(ctx, sqlc.ListActivityParams{
			BusinessID: businessID, BusinessID_2: businessID, BusinessID_3: businessID, BusinessID_4: businessID,
			ActorID: pgUUIDOrNil(f.ActorID), ActivityType: pgTextOrNil(f.Type),
			StartAt: pgTimestamptzOrNil(f.StartAt), EndAt: pgTimestamptzOrNil(f.EndAt),
			RowLimit: limit,
		})
		if err != nil {
			return err
		}
		rows = r
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list activity: %w", err)
	}
	out := make([]Entry, 0, len(rows))
	for _, r := range rows {
		out = append(out, toEntry(r))
	}
	return out, nil
}

// CountByDay backs the Activity page's heatmap: one count per local
// calendar day (bucketed via timezone, docs/ARCHITECTURE.md §10.2/§14),
// merged across all four activity sources. Four separate, single-table
// queries rather than one UNION+GROUP BY -- see internal/store/queries/
// activity.sql's comment on why the combined form tripped sqlc's static
// analyzer.
func (s *Service) CountByDay(ctx context.Context, userID, businessID uuid.UUID, timezone string, start, end time.Time) ([]DayCount, error) {
	counts := make(map[time.Time]int64)
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		startTz, endTz := pgTimestamptzOrNil(start), pgTimestamptzOrNil(end)

		sales, err := q.CountSalesByDay(ctx, sqlc.CountSalesByDayParams{
			BusinessID: businessID, Column2: timezone, OccurredAt: startTz, OccurredAt_2: endTz,
		})
		if err != nil {
			return fmt.Errorf("count sales by day: %w", err)
		}
		for _, r := range sales {
			counts[r.Day.Time] += r.N
		}

		exp, err := q.CountExpensesByDay(ctx, sqlc.CountExpensesByDayParams{
			BusinessID: businessID, Column2: timezone, OccurredAt: startTz, OccurredAt_2: endTz,
		})
		if err != nil {
			return fmt.Errorf("count expenses by day: %w", err)
		}
		for _, r := range exp {
			counts[r.Day.Time] += r.N
		}

		adj, err := q.CountApprovedAdjustmentsByDay(ctx, sqlc.CountApprovedAdjustmentsByDayParams{
			BusinessID: businessID, Column2: timezone, CreatedAt: startTz, CreatedAt_2: endTz,
		})
		if err != nil {
			return fmt.Errorf("count approved adjustments by day: %w", err)
		}
		for _, r := range adj {
			counts[r.Day.Time] += r.N
		}

		receipts, err := q.CountStockReceiptsByDay(ctx, sqlc.CountStockReceiptsByDayParams{
			BusinessID: businessID, Column2: timezone, ReceivedAt: startTz, ReceivedAt_2: endTz,
		})
		if err != nil {
			return fmt.Errorf("count stock receipts by day: %w", err)
		}
		for _, r := range receipts {
			counts[r.Day.Time] += r.N
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	out := make([]DayCount, 0, len(counts))
	for day, n := range counts {
		out = append(out, DayCount{Day: day, Count: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day.Before(out[j].Day) })
	return out, nil
}

func newID() (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("generate id: %w", err)
	}
	return id, nil
}

func toFlag(f sqlc.ActivityFlag) Flag {
	return Flag{
		ID: f.ID, SourceType: f.SourceType, SourceID: f.SourceID, FlaggedBy: f.FlaggedBy,
		Reason: f.Reason, Status: f.Status, ResolvedBy: toUUIDOrNil(f.ResolvedBy),
		ResolutionNote: pgTextValue(f.ResolutionNote), CreatedAt: f.CreatedAt.Time,
		ResolvedAt: f.ResolvedAt.Time,
	}
}

func toUUIDOrNil(id pgtype.UUID) uuid.UUID {
	if !id.Valid {
		return uuid.Nil
	}
	return id.Bytes
}

func pgTextValue(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

// CreateFlag records a staff (or owner) concern about one activity entry.
// Callers must have already verified sourceID actually exists and that the
// caller is allowed to see it (internal/httpapi's getSale/getExpense/
// getAdjustmentRequest/getStockReceipt handlers already do this lookup
// before flagging, so it is never redone here).
func (s *Service) CreateFlag(ctx context.Context, userID, businessID uuid.UUID, sourceType string, sourceID uuid.UUID, reason string) (Flag, error) {
	id, err := newID()
	if err != nil {
		return Flag{}, err
	}
	var created sqlc.ActivityFlag
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.CreateActivityFlag(ctx, sqlc.CreateActivityFlagParams{
			ID: id, BusinessID: businessID, SourceType: sourceType, SourceID: sourceID,
			FlaggedBy: userID, Reason: reason,
		})
		if err != nil {
			return err
		}
		created = row
		return nil
	})
	if err != nil {
		return Flag{}, fmt.Errorf("create activity flag: %w", err)
	}
	return toFlag(created), nil
}

// ListOpenFlags returns every unresolved flag, newest first -- surfaced in
// Reviews alongside the existing sale/inventory reviews.
func (s *Service) ListOpenFlags(ctx context.Context, userID, businessID uuid.UUID) ([]Flag, error) {
	var rows []sqlc.ActivityFlag
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		r, err := q.ListOpenActivityFlags(ctx, businessID)
		if err != nil {
			return err
		}
		rows = r
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list open activity flags: %w", err)
	}
	out := make([]Flag, 0, len(rows))
	for _, r := range rows {
		out = append(out, toFlag(r))
	}
	return out, nil
}

// ResolveFlag marks a flag resolved with the owner's note. It never touches
// the record the flag points at -- an owner who agrees something needs
// fixing performs that fix through the record's own correction mechanism
// separately (a reversal, or a new adjustment), same "resolve the
// decision, never silently alter the thing it's about" shape as
// sale_reviews/inventory_reviews.
func (s *Service) ResolveFlag(ctx context.Context, userID, businessID, flagID, resolvedBy uuid.UUID, note string) (Flag, error) {
	var updated sqlc.ActivityFlag
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.ResolveActivityFlag(ctx, sqlc.ResolveActivityFlagParams{
			BusinessID: businessID, ID: flagID, ResolvedBy: pgUUIDOrNil(resolvedBy), ResolutionNote: pgTextOrNil(note),
		})
		if err != nil {
			return err
		}
		updated = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Flag{}, ErrFlagNotFound
		}
		return Flag{}, fmt.Errorf("resolve activity flag: %w", err)
	}
	return toFlag(updated), nil
}

// GetFlag returns one flag, any status -- used to verify a flag's own
// business ownership before responding to it (e.g. activity_handlers.go's
// resolveActivityFlag error mapping).
func (s *Service) GetFlag(ctx context.Context, userID, businessID, flagID uuid.UUID) (Flag, error) {
	var found sqlc.ActivityFlag
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.GetActivityFlagByID(ctx, sqlc.GetActivityFlagByIDParams{BusinessID: businessID, ID: flagID})
		if err != nil {
			return err
		}
		found = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Flag{}, ErrFlagNotFound
		}
		return Flag{}, fmt.Errorf("get activity flag: %w", err)
	}
	return toFlag(found), nil
}
