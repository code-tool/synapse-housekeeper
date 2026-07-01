package processor

import (
	"context"
	"sync"
	"time"

	"maunium.net/go/mautrix/id"
)

// RoomPurgeScheduleMemory is an in-memory RoomPurgeScheduleStore. It is safe for
// concurrent use by the cleanup workers but does not survive process restarts.
type RoomPurgeScheduleMemory struct {
	mu      sync.Mutex
	records map[id.RoomID]time.Time
}

func NewRoomPurgeScheduleMemory() *RoomPurgeScheduleMemory {
	return &RoomPurgeScheduleMemory{records: make(map[id.RoomID]time.Time)}
}

func (m *RoomPurgeScheduleMemory) Get(_ context.Context, roomID id.RoomID) (*RoomPurgeSchedule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	purgeAfter, ok := m.records[roomID]
	if !ok {
		return nil, nil
	}

	return &RoomPurgeSchedule{RoomID: roomID, PurgeAfter: purgeAfter}, nil
}

func (m *RoomPurgeScheduleMemory) Schedule(_ context.Context, roomID id.RoomID, purgeAfter time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Set once: do not overwrite an existing record (mirrors the Postgres
	// INSERT ... ON CONFLICT DO NOTHING semantics).
	if _, ok := m.records[roomID]; ok {
		return nil
	}
	m.records[roomID] = purgeAfter

	return nil
}

func (m *RoomPurgeScheduleMemory) Delete(_ context.Context, roomID id.RoomID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.records, roomID)

	return nil
}

func (m *RoomPurgeScheduleMemory) Close() error {
	return nil
}
