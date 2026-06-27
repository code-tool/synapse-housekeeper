package processor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"maunium.net/go/mautrix/id"
)

type RoomPurgeSchedulePostgres struct {
	pool *pgxpool.Pool
}

func NewRoomPurgeSchedulePostgres(ctx context.Context, connString string) (*RoomPurgeSchedulePostgres, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	store := &RoomPurgeSchedulePostgres{pool: pool}
	if err := store.ensureSchema(ctx); err != nil {
		pool.Close()

		return nil, err
	}

	return store, nil
}

func (s *RoomPurgeSchedulePostgres) Close() error {
	s.pool.Close()

	return nil
}

func (s *RoomPurgeSchedulePostgres) ensureSchema(ctx context.Context) error {
	const query = `
CREATE TABLE IF NOT EXISTS synapse_room_purge_schedule (
	room_id     text PRIMARY KEY,
	purge_after timestamptz NOT NULL,
	created_at  timestamptz NOT NULL DEFAULT now()
);`

	if _, err := s.pool.Exec(ctx, query); err != nil {
		return fmt.Errorf("ensure room purge schedule schema: %w", err)
	}

	return nil
}

func (s *RoomPurgeSchedulePostgres) Get(ctx context.Context, roomID id.RoomID) (*RoomPurgeSchedule, error) {
	const query = `
SELECT room_id, purge_after
FROM synapse_room_purge_schedule
WHERE room_id = $1;`

	var (
		schedule  RoomPurgeSchedule
		roomIDStr string
	)
	err := s.pool.QueryRow(ctx, query, roomID.String()).Scan(&roomIDStr, &schedule.PurgeAfter)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	schedule.RoomID = id.RoomID(roomIDStr)

	return &schedule, nil
}

func (s *RoomPurgeSchedulePostgres) Schedule(ctx context.Context, roomID id.RoomID, purgeAfter time.Time) error {
	const query = `
INSERT INTO synapse_room_purge_schedule (room_id, purge_after)
VALUES ($1, $2)
ON CONFLICT (room_id) DO NOTHING;`

	_, err := s.pool.Exec(ctx, query, roomID.String(), purgeAfter)

	return err
}

func (s *RoomPurgeSchedulePostgres) Delete(ctx context.Context, roomID id.RoomID) error {
	const query = `DELETE FROM synapse_room_purge_schedule WHERE room_id = $1;`

	_, err := s.pool.Exec(ctx, query, roomID.String())

	return err
}
