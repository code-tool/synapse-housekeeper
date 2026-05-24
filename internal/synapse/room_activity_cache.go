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
}

type RoomActivityCacheNull struct{}

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

func (RoomActivityCacheNull) RoomActivity(ctx context.Context, roomID id.RoomID) (*RoomActivityCacheEntry, error) {
	return nil, nil
}

func (RoomActivityCacheNull) StoreRoomActivity(ctx context.Context, entry RoomActivityCacheEntry) error {
	return nil
}

type noopCloser struct{}

func (noopCloser) Close() error {
	return nil
}
