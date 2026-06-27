package synapse

import (
	"context"
	"time"

	"maunium.net/go/mautrix/id"
)

type RoomPurgeScheduleNull struct{}

func (RoomPurgeScheduleNull) Get(_ context.Context, _ id.RoomID) (*RoomPurgeSchedule, error) {
	return nil, nil
}

func (RoomPurgeScheduleNull) Schedule(_ context.Context, _ id.RoomID, _ time.Time) error {
	return nil
}

func (RoomPurgeScheduleNull) Delete(_ context.Context, _ id.RoomID) error {
	return nil
}
