package synapse

import (
	"context"
	"io"
	"strings"
	"time"

	"maunium.net/go/mautrix/id"
)

type RoomActivityCache interface {
	LastMessageAt(ctx context.Context, roomID id.RoomID) (time.Time, bool, error)
	StoreLastMessageAt(ctx context.Context, roomID id.RoomID, lastMessageAt time.Time) error
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

func (RoomActivityCacheNull) LastMessageAt(ctx context.Context, roomID id.RoomID) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

func (RoomActivityCacheNull) StoreLastMessageAt(ctx context.Context, roomID id.RoomID, lastMessageAt time.Time) error {
	return nil
}

type noopCloser struct{}

func (noopCloser) Close() error {
	return nil
}
