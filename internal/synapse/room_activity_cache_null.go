package synapse

import (
	"context"
	"io"
	"strings"
	"time"

	"maunium.net/go/mautrix/id"
)

type noopCloser struct{}

func (noopCloser) Close() error {
	return nil
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

func (RoomActivityCacheNull) DeleteCandidateEntries(_ context.Context, _ time.Time) error {
	return nil
}
