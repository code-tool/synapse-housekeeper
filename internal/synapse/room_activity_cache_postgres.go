package synapse

import (
	"context"
	"errors"
	"fmt"

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

func (c *RoomActivityCachePostgres) Close() error {
	c.pool.Close()

	return nil
}

func (c *RoomActivityCachePostgres) ensureSchema(ctx context.Context) error {
	const query = `
CREATE TABLE IF NOT EXISTS synapse_room_activity_cache (
	room_id text PRIMARY KEY,
	last_message_at timestamptz NOT NULL,
	joined_members integer NOT NULL DEFAULT 0,
	updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE synapse_room_activity_cache
ADD COLUMN IF NOT EXISTS joined_members integer NOT NULL DEFAULT 0;`

	if _, err := c.pool.Exec(ctx, query); err != nil {
		return fmt.Errorf("ensure room activity cache schema: %w", err)
	}

	return nil
}

func (c *RoomActivityCachePostgres) RoomActivity(ctx context.Context, roomID id.RoomID) (*RoomActivityCacheEntry, error) {
	const query = `
SELECT room_id, last_message_at, joined_members, updated_at
FROM synapse_room_activity_cache
WHERE room_id = $1;`

	var (
		entry     RoomActivityCacheEntry
		roomIDStr string
	)
	err := c.pool.QueryRow(ctx, query, roomID.String()).
		Scan(&roomIDStr, &entry.LastMessageAt, &entry.JoinedMembers, &entry.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	entry.RoomID = id.RoomID(roomIDStr)

	return &entry, nil
}

func (c *RoomActivityCachePostgres) StoreRoomActivity(ctx context.Context, entry RoomActivityCacheEntry) error {
	const query = `
INSERT INTO synapse_room_activity_cache (room_id, last_message_at, joined_members, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (room_id) DO UPDATE
SET
	last_message_at = EXCLUDED.last_message_at,
	joined_members = EXCLUDED.joined_members,
	updated_at = now();`

	_, err := c.pool.Exec(ctx, query, entry.RoomID.String(), entry.LastMessageAt, entry.JoinedMembers)

	return err
}
