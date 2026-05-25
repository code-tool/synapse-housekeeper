package synapse

import (
	"context"
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
