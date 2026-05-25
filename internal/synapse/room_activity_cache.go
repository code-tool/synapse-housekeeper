package synapse

import (
	"context"
	"io"
	"strings"
	"time"

	"maunium.net/go/mautrix/id"
)

type RoomActivityCacheEntry struct {
	RoomID        id.RoomID
	LastMessageAt time.Time
	JoinedMembers int
	UpdatedAt     time.Time
}

type RoomActivityCache interface {
	RoomActivity(ctx context.Context, roomID id.RoomID) (*RoomActivityCacheEntry, error)
	StoreRoomActivity(ctx context.Context, entry RoomActivityCacheEntry) error
	DeleteCandidateEntries(ctx context.Context, abandonedBefore time.Time) error
}

func NewRoomActivityCache(ctx context.Context, dsn string) (RoomActivityCache, io.Closer, error) {
	if strings.TrimSpace(dsn) == "" {
		return RoomActivityCacheNull{}, noopCloser{}, nil
	}

	cache, err := NewRoomActivityCachePostgres(ctx, dsn)
	if err != nil {
		return nil, nil, err
	}

	return cache, cache, nil
}
