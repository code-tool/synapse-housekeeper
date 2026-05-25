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
	"maunium.net/go/mautrix/synapseadmin"

	"synapse-housekeeper/internal/synapse"
)

type RoomCleaner struct {
	log           *zap.Logger
	synapseClient *synapse.Client
	iterator      *synapse.RoomCleanupIterator

	workersCount int
}

type RoomCleanerStatistics struct {
	Processed       int64
	Empty           int64
	NoMessages      int64
	AbandonedSingle int64
	AbandonedMulti  int64
}

func NewRoomCleaner(log *zap.Logger, synapseClient *synapse.Client, iterator *synapse.RoomCleanupIterator, workersCount int) *RoomCleaner {
	return &RoomCleaner{log: log, synapseClient: synapseClient, iterator: iterator, workersCount: workersCount}
}

func (r *RoomCleaner) purgeRoom(ctx context.Context, doRealJob bool, roomInfo *synapseadmin.RoomInfo) error {
	if !doRealJob {
		return nil
	}

	r.log.Info("deleting room",
		zap.Stringer("room_id", roomInfo.RoomID),
		zap.Int("joined_members", roomInfo.JoinedMembers))

	_, err := r.synapseClient.DeleteRoom(ctx, roomInfo.RoomID, synapseadmin.ReqDeleteRoom{Purge: true})
	if err == nil {
		return nil
	}

	if httpErr, ok := errors.AsType[mautrix.HTTPError](err); ok && httpErr.IsStatus(400) {
		if dStatusResp, err := r.synapseClient.DeleteStatus(ctx, roomInfo.RoomID); err == nil {
			r.log.Warn("room delete already scheduled", zap.String("status", dStatusResp.Results[0].Status))
		}
		return nil
	}

	return fmt.Errorf("can't delete room: %w", err)
}

func (r *RoomCleaner) recordCandidate(stat *RoomCleanerStatistics, candidate synapse.RoomCleanupCandidate) {
	switch candidate.Reason {
	case synapse.RoomCleanupReasonEmpty:
		atomic.AddInt64(&stat.Empty, 1)

	case synapse.RoomCleanupReasonNoMessages:
		atomic.AddInt64(&stat.NoMessages, 1)
		r.log.Debug("Room without messages",
			zap.Stringer("room_id", candidate.Room.RoomID),
			zap.Int("joined_members", candidate.Room.JoinedMembers),
		)

	case synapse.RoomCleanupReasonAbandoned:
		r.log.Debug("found abandoned room",
			zap.Stringer("room_id", candidate.Room.RoomID),
			zap.Int("joined_members", candidate.Room.JoinedMembers),
			zap.String("since_last_event", humanize.Time(candidate.LastMessageAt)),
		)

		if candidate.Room.JoinedMembers > 1 {
			atomic.AddInt64(&stat.AbandonedMulti, 1)
		} else {
			atomic.AddInt64(&stat.AbandonedSingle, 1)
		}
	}
}

func (r *RoomCleaner) worker(
	ctx context.Context,
	doRealJob bool, stat *RoomCleanerStatistics,
	jobs <-chan synapse.RoomCleanupCandidate,
) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case candidate, ok := <-jobs:
			if !ok {
				return nil
			}

			r.recordCandidate(stat, candidate)

			if err := r.purgeRoom(ctx, doRealJob, &candidate.Room); err != nil {
				return err
			}
		}
	}
}

func (r *RoomCleaner) Process(ctx context.Context, doRealJob bool, abandonedBefore time.Time, noCacheCleanup bool) error {
	lRoomReq := synapseadmin.ReqListRoom{
		Direction: mautrix.DirectionBackward,
		OrderBy:   "joined_members",
		Limit:     1000,
	}

	stat := &RoomCleanerStatistics{}
	logStats := func() {
		r.log.Info("statistics",
			zap.Int64("processed", atomic.LoadInt64(&stat.Processed)),
			zap.Int64("empty", atomic.LoadInt64(&stat.Empty)),
			zap.Int64("no_messages", atomic.LoadInt64(&stat.NoMessages)),
			zap.Int64("abandoned_single", atomic.LoadInt64(&stat.AbandonedSingle)),
			zap.Int64("abandoned_multi", atomic.LoadInt64(&stat.AbandonedMulti)),
		)
	}
	defer logStats()

	errG, ctx := errgroup.WithContext(ctx)
	roomInfoChan := make(chan synapse.RoomCleanupCandidate)

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

	itErr := r.iterator.Iterate(ctx, synapse.RoomCleanupCandidateOptions{
		AbandonedBefore: abandonedBefore,
		NoCacheCleanup:  noCacheCleanup,
		ListRequest:     lRoomReq,
		Workers:         r.workersCount,
		OnRoomChecked: func(ctx context.Context, roomInfo synapseadmin.RoomInfo) {
			atomic.AddInt64(&stat.Processed, 1)
		},
		OnRoomError: func(ctx context.Context, roomInfo synapseadmin.RoomInfo, err error) bool {
			r.log.Error("Failed to check room status", zap.String("room_id", string(roomInfo.RoomID)), zap.Error(err))

			return true
		},
	}, func(ctx context.Context, candidate synapse.RoomCleanupCandidate) bool {
		select {
		case <-ctx.Done():
			return false
		case roomInfoChan <- candidate:
			return true
		}
	})

	close(roomInfoChan)
	err := errG.Wait()

	return errors.Join(itErr, err)
}
