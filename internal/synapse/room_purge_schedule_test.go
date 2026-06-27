package synapse

import (
	"context"
	"testing"
	"time"
)

func TestRoomPurgeScheduleNull(t *testing.T) {
	ctx := context.Background()
	var store RoomPurgeScheduleStore = RoomPurgeScheduleNull{}

	rec, err := store.Get(ctx, "!room:test")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if rec != nil {
		t.Fatalf("Get() = %v, want nil", rec)
	}
	if err := store.Schedule(ctx, "!room:test", time.Now()); err != nil {
		t.Fatalf("Schedule() error = %v, want nil", err)
	}
	if err := store.Delete(ctx, "!room:test"); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}
}

func TestNewRoomPurgeScheduleStoreBlankDSN(t *testing.T) {
	store, closer, err := NewRoomPurgeScheduleStore(context.Background(), "  ")
	if err != nil {
		t.Fatalf("NewRoomPurgeScheduleStore() error = %v, want nil", err)
	}
	if _, ok := store.(RoomPurgeScheduleNull); !ok {
		t.Fatalf("store type = %T, want RoomPurgeScheduleNull", store)
	}
	if err := closer.Close(); err != nil {
		t.Fatalf("closer.Close() error = %v, want nil", err)
	}
}
