package synapse

import (
	"context"
	"io"
	"strings"
	"time"

	"maunium.net/go/mautrix/id"
)

// RoomPurgeSchedule records when a soft-deleted room becomes eligible for a
// full purge (purge=true).
type RoomPurgeSchedule struct {
	RoomID     id.RoomID
	PurgeAfter time.Time
}

type RoomPurgeScheduleStore interface {
	// Get returns the schedule for roomID, or (nil, nil) when none exists.
	Get(ctx context.Context, roomID id.RoomID) (*RoomPurgeSchedule, error)
	// Schedule records purgeAfter for roomID. It does not overwrite an
	// existing record (the cooldown is set once, when the room is soft-deleted).
	Schedule(ctx context.Context, roomID id.RoomID, purgeAfter time.Time) error
	// Delete removes the schedule for roomID after a successful full purge.
	Delete(ctx context.Context, roomID id.RoomID) error
}

func NewRoomPurgeScheduleStore(ctx context.Context, dsn string) (RoomPurgeScheduleStore, io.Closer, error) {
	if strings.TrimSpace(dsn) == "" {
		return RoomPurgeScheduleNull{}, noopCloser{}, nil
	}

	store, err := NewRoomPurgeSchedulePostgres(ctx, dsn)
	if err != nil {
		return nil, nil, err
	}

	return store, store, nil
}
