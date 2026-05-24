package synapse

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"maunium.net/go/mautrix/id"
)

type RoomActivityCachePostgres struct {
	pool *pgxpool.Pool
}

func NewRoomActivityCachePostgres(ctx context.Context, connString string) (*RoomActivityCachePostgres, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	cache := &RoomActivityCachePostgres{pool: pool}
	if err := cache.ensureSchema(ctx); err != nil {
		pool.Close()

		return nil, err
	}

	return cache, nil
}

func (c *RoomActivityCachePostgres) Close() {
	c.pool.Close()
}

func (c *RoomActivityCachePostgres) ensureSchema(ctx context.Context) error {
	const query = `
CREATE TABLE IF NOT EXISTS synapse_room_activity_cache (
	room_id text PRIMARY KEY,
	last_message_at timestamptz NOT NULL,
	updated_at timestamptz NOT NULL DEFAULT now()
);`

	if _, err := c.pool.Exec(ctx, query); err != nil {
		return fmt.Errorf("ensure room activity cache schema: %w", err)
	}

	return nil
}

func (c *RoomActivityCachePostgres) LastMessageAt(ctx context.Context, roomID id.RoomID) (time.Time, bool, error) {
	const query = `
SELECT last_message_at
FROM synapse_room_activity_cache
WHERE room_id = $1;`

	var lastMessageAt time.Time
	err := c.pool.QueryRow(ctx, query, roomID.String()).Scan(&lastMessageAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}

	return lastMessageAt, true, nil
}

func (c *RoomActivityCachePostgres) StoreLastMessageAt(ctx context.Context, roomID id.RoomID, lastMessageAt time.Time) error {
	const query = `
INSERT INTO synapse_room_activity_cache (room_id, last_message_at, updated_at)
VALUES ($1, $2, now())
ON CONFLICT (room_id) DO UPDATE
SET
	last_message_at = EXCLUDED.last_message_at,
	updated_at = now();`

	_, err := c.pool.Exec(ctx, query, roomID.String(), lastMessageAt)

	return err
}
