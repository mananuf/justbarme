package activity

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
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
