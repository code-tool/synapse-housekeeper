package synapse

import (
	"context"
	"time"

	"maunium.net/go/mautrix/id"
)

type noopCloser struct{}

func (noopCloser) Close() error {
	return nil
}

type RoomActivityCacheNull struct{}

func (RoomActivityCacheNull) RoomActivity(ctx context.Context, roomID id.RoomID) (*RoomActivityCacheEntry, error) {
	return nil, nil
}

func (RoomActivityCacheNull) StoreRoomActivity(ctx context.Context, entry RoomActivityCacheEntry) error {
	return nil
}

func (RoomActivityCacheNull) DeleteCandidateEntries(_ context.Context, _ time.Time) error {
	return nil
}
