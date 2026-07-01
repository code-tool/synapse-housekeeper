package processor

import (
	"context"
	"testing"
	"time"

	"maunium.net/go/mautrix/id"
)

func TestRoomPurgeScheduleMemory(t *testing.T) {
	ctx := context.Background()
	store := NewRoomPurgeScheduleMemory()
	roomID := id.RoomID("!room:test")
	purgeAfter := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)

	// Empty store returns no record.
	rec, err := store.Get(ctx, roomID)
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if rec != nil {
		t.Fatalf("Get() = %v, want nil", rec)
	}

	// After Schedule, Get returns the recorded schedule.
	if err := store.Schedule(ctx, roomID, purgeAfter); err != nil {
		t.Fatalf("Schedule() error = %v, want nil", err)
	}
	rec, err = store.Get(ctx, roomID)
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if rec == nil || rec.RoomID != roomID || !rec.PurgeAfter.Equal(purgeAfter) {
		t.Fatalf("Get() = %v, want {%s %v}", rec, roomID, purgeAfter)
	}

	// Schedule is set-once: a second call does not overwrite the cooldown.
	if err := store.Schedule(ctx, roomID, purgeAfter.Add(time.Hour)); err != nil {
		t.Fatalf("Schedule() error = %v, want nil", err)
	}
	rec, _ = store.Get(ctx, roomID)
	if rec == nil || !rec.PurgeAfter.Equal(purgeAfter) {
		t.Fatalf("Schedule overwrote the cooldown: got %v, want %v", rec, purgeAfter)
	}

	// Delete removes the record.
	if err := store.Delete(ctx, roomID); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}
	rec, _ = store.Get(ctx, roomID)
	if rec != nil {
		t.Fatalf("Get() after Delete = %v, want nil", rec)
	}
}

func TestNewRoomPurgeScheduleStoreBlankDSN(t *testing.T) {
	store, closer, err := NewRoomPurgeScheduleStore(context.Background(), "  ")
	if err != nil {
		t.Fatalf("NewRoomPurgeScheduleStore() error = %v, want nil", err)
	}
	if _, ok := store.(*RoomPurgeScheduleMemory); !ok {
		t.Fatalf("store type = %T, want *RoomPurgeScheduleMemory", store)
	}
	if err := closer.Close(); err != nil {
		t.Fatalf("closer.Close() error = %v, want nil", err)
	}
}
