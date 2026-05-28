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
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/synapseadmin"

	"synapse-housekeeper/internal/synapse"
)

type RoomCleaner struct {
	log           *zap.Logger
	synapseClient roomCleanerClient
	iterator      roomCleanupIterator

	workersCount int
}

type roomCleanerClient interface {
	DeleteRoom(ctx context.Context, roomID id.RoomID, req synapseadmin.ReqDeleteRoom) (synapseadmin.RespDeleteRoom, error)
	DeleteStatus(ctx context.Context, roomID id.RoomID) (synapse.RespDeleteStatus, error)
}

type roomCleanupIterator interface {
	Iterate(
		ctx context.Context,
		opts synapse.RoomCleanupCandidateOptions,
		yield func(ctx context.Context, candidate synapse.RoomCleanupCandidate) bool,
	) error
}

type RoomCleanerStatistics struct {
	Processed     int64
	Empty         int64
	NoMessages    int64
	AbandonedOne  int64
	AbandonedPair int64
	AbandonedMany int64
}

func NewRoomCleaner(log *zap.Logger, synapseClient roomCleanerClient, iterator roomCleanupIterator, workersCount int) *RoomCleaner {
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
			if len(dStatusResp.Results) > 0 {
				r.log.Warn("room delete already scheduled", zap.String("status", dStatusResp.Results[0].Status))
				return nil
			}
		}
	}

	return fmt.Errorf("can't delete room: %w", err)
}

func (r *RoomCleaner) recordCandidate(stat *RoomCleanerStatistics, candidate synapse.RoomCleanupCandidate) {
	log := r.log.With(zap.Stringer("room_id", candidate.Room.RoomID))
	if candidate.Room.Name != "" {
		log = log.With(zap.String("room_name", candidate.Room.Name))
	}
	log = log.With(zap.Int("joined_members", candidate.Room.JoinedMembers))

	switch candidate.Reason {
	case synapse.RoomCleanupReasonEmpty:
		atomic.AddInt64(&stat.Empty, 1)
		log.Debug("Empty room")

	case synapse.RoomCleanupReasonNoMessages:
		atomic.AddInt64(&stat.NoMessages, 1)
		log.Debug("Room without messages")

	case synapse.RoomCleanupReasonAbandoned:
		log.Debug("Abandoned room", zap.String("since_last_event", humanize.Time(candidate.LastMessageAt)))

		switch candidate.Room.JoinedMembers {
		case 0:
			panic("joined_members == 0 in abandoned room")
		case 1:
			atomic.AddInt64(&stat.AbandonedOne, 1)
		case 2:
			atomic.AddInt64(&stat.AbandonedPair, 1)
		default:
			atomic.AddInt64(&stat.AbandonedMany, 1)
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

type RoomCleanerOptions struct {
	DoRealJob           bool
	AbandonedBefore     time.Time
	NoCacheCleanup      bool
	FilterOnlyForUserID id.UserID
}

func (r *RoomCleaner) Process(ctx context.Context, opts RoomCleanerOptions) error {
	stat := &RoomCleanerStatistics{}
	logStats := func() {
		r.log.Info("statistics",
			zap.Int64("processed", atomic.LoadInt64(&stat.Processed)),
			zap.Int64("empty", atomic.LoadInt64(&stat.Empty)),
			zap.Int64("no_messages", atomic.LoadInt64(&stat.NoMessages)),
			zap.Object("abandoned", zap.DictObject(
				zap.Int64("one", atomic.LoadInt64(&stat.AbandonedOne)),
				zap.Int64("pair", atomic.LoadInt64(&stat.AbandonedPair)),
				zap.Int64("many", atomic.LoadInt64(&stat.AbandonedMany)),
			)),
		)
	}
	defer logStats()

	errG, ctx := errgroup.WithContext(ctx)
	roomInfoChan := make(chan synapse.RoomCleanupCandidate)

	for i := 0; i < r.workersCount; i++ {
		errG.Go(func() error {
			return r.worker(ctx, opts.DoRealJob, stat, roomInfoChan)
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
		AbandonedBefore:     opts.AbandonedBefore,
		NoCacheCleanup:      opts.NoCacheCleanup,
		Workers:             r.workersCount,
		FilterOnlyForUserID: opts.FilterOnlyForUserID,
		OnRoomChecked: func(ctx context.Context, roomInfo synapseadmin.RoomInfo) {
			atomic.AddInt64(&stat.Processed, 1)
		},
		OnRoomError: func(ctx context.Context, roomInfo synapseadmin.RoomInfo, err error) bool {
			if !errors.Is(err, context.Canceled) {
				r.log.Error("Failed to check room status", zap.String("room_id", string(roomInfo.RoomID)), zap.Error(err))
			}

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
