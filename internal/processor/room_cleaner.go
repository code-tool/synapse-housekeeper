package processor

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/dustin/go-humanize"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/synapseadmin"

	"synapse-housekeeper/internal/synapse"
)

type RoomCleaner struct {
	log           *zap.Logger
	synapseClient *synapse.Client

	workersCount int
}

type RoomCleanerStatistics struct {
	Processed       int64
	Empty           int64
	AbandonedSingle int64
	AbandonedMulti  int64
}

func NewRoomCleaner(log *zap.Logger, synapseClient *synapse.Client) *RoomCleaner {
	return &RoomCleaner{log: log, synapseClient: synapseClient, workersCount: 5}
}

func (r *RoomCleaner) purgeRoom(ctx context.Context, doRealJob bool, roomId id.RoomID) error {
	if !doRealJob {
		return nil
	}

	r.log.Info("deleting room", zap.Stringer("room_id", roomId))

	_, err := r.synapseClient.DeleteRoom(ctx, roomId, synapseadmin.ReqDeleteRoom{Purge: true})
	if err == nil {
		return nil
	}

	if httpErr, ok := errors.AsType[mautrix.HTTPError](err); ok && httpErr.IsStatus(400) {
		if dStatusResp, err := r.synapseClient.DeleteStatus(ctx, roomId); err == nil {
			r.log.Warn("room delete already scheduled", zap.String("status", dStatusResp.Results[0].Status))
		}
		return nil
	}

	return fmt.Errorf("can't delete room: %w", err)
}

func (r *RoomCleaner) getLastRoomMessageTs(ctx context.Context, roomID id.RoomID) (time.Time, error) {
	now := time.Now()
	tsToEventResp, err := r.synapseClient.AdminTimestampToEvent(ctx, roomID, now, mautrix.DirectionBackward)
	if err != nil {
		return time.Time{}, fmt.Errorf("timestamp to event: %w", err)
	}

	ctxResp, err := r.synapseClient.AdminContext(ctx, roomID, tsToEventResp.EventID, nil, 0)
	if err != nil {
		return time.Time{}, fmt.Errorf("admin event context: %w", err)
	}

	filters := &mautrix.FilterPart{
		Types: []event.Type{
			event.EventRedaction, event.EventMessage,
			event.EventEncrypted, event.EventReaction,
		},
	}
	messageResp, err := r.synapseClient.
		RoomMessages(ctx, roomID, ctxResp.End, "", mautrix.DirectionBackward, filters, 1)
	if err != nil {
		return time.Time{}, fmt.Errorf("room messages: %w", err)
	}

	if len(messageResp.Chunk) == 0 {
		return time.Time{}, nil // no messages found
	}

	return time.UnixMilli(messageResp.Chunk[0].Timestamp), nil
}

func (r *RoomCleaner) isRoomShouldBeDeleted(ctx context.Context, stat *RoomCleanerStatistics, roomInfo synapseadmin.RoomInfo) (bool, error) {
	atomic.AddInt64(&stat.Processed, 1)

	if roomInfo.JoinedMembers <= 0 {
		atomic.AddInt64(&stat.Empty, 1)

		return true, nil
	}

	lastMessageTs, err := r.getLastRoomMessageTs(ctx, roomInfo.RoomID)
	if err != nil {
		return false, err
	}

	if lastMessageTs.IsZero() {
		r.log.Info("Room without messages",
			zap.Stringer("room_id", roomInfo.RoomID),
			zap.Int("joined_members", roomInfo.JoinedMembers),
		)

		return true, nil // no messages found
	}

	const day = 24 * time.Hour
	sinceLastMessage := time.Since(lastMessageTs)

	if sinceLastMessage > (365+45)*day {
		r.log.Info("found abandoned room",
			zap.Stringer("room_id", roomInfo.RoomID),
			zap.Int("joined_members", roomInfo.JoinedMembers),
			zap.String("since_last_event", humanize.Time(lastMessageTs)),
		)

		if roomInfo.JoinedMembers > 1 {
			atomic.AddInt64(&stat.AbandonedMulti, 1)
		} else {
			atomic.AddInt64(&stat.AbandonedSingle, 1)
		}

		return true, nil
	}

	return false, nil
}

func (r *RoomCleaner) worker(
	ctx context.Context,
	doRealJob bool, stat *RoomCleanerStatistics,
	jobs <-chan synapseadmin.RoomInfo,
) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case roomInfo, ok := <-jobs:
			if !ok {
				return nil
			}

			shouldBeDeleted, err := r.isRoomShouldBeDeleted(ctx, stat, roomInfo)
			if err != nil {
				r.log.Error("Failed to check room status", zap.String("room_id", string(roomInfo.RoomID)), zap.Error(err))

				continue
				// return err
			}

			if !shouldBeDeleted {
				continue
			}

			if err := r.purgeRoom(ctx, doRealJob, roomInfo.RoomID); err != nil {
				return err
			}
		}
	}
}

func (r *RoomCleaner) Process(ctx context.Context, doRealJob bool) error {
	lRoomReq := synapseadmin.ReqListRoom{
		Direction: mautrix.DirectionBackward,
		OrderBy:   "joined_members",
		Limit:     1000,
	}

	stat := &RoomCleanerStatistics{}
	logStats := func() {
		r.log.Info("statistics",
			zap.Int64("processed", stat.Processed),
			zap.Int64("empty", stat.Empty),
			zap.Int64("abandoned_single", stat.AbandonedSingle),
			zap.Int64("abandoned_multi", stat.AbandonedMulti),
		)
	}
	defer logStats()

	errG, ctx := errgroup.WithContext(ctx)
	roomInfoChan := make(chan synapseadmin.RoomInfo)

	for i := 0; i < r.workersCount; i++ {
		errG.Go(func() error {
			return r.worker(ctx, doRealJob, stat, roomInfoChan)
		})
	}

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				logStats()
			case <-ctx.Done():
				return
			}
		}
	}()

	itErr := r.synapseClient.ListRoomsIt(ctx, lRoomReq, func(ctx context.Context, roomInfo synapseadmin.RoomInfo) bool {
		//if roomInfo.JoinedMembers > 1 {
		//	return false
		//}

		select {
		case <-ctx.Done():
			return false
		case roomInfoChan <- roomInfo:
			return true
		}
	})

	close(roomInfoChan)
	err := errG.Wait()

	return errors.Join(itErr, err)
}
