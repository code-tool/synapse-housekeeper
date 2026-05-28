package synapse

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/synapseadmin"
)

type RoomCleanupReason string

const (
	RoomCleanupReasonEmpty      RoomCleanupReason = "empty"
	RoomCleanupReasonNoMessages RoomCleanupReason = "no_messages"
	RoomCleanupReasonAbandoned  RoomCleanupReason = "abandoned"
)

type RoomCleanupCandidate struct {
	Room          synapseadmin.RoomInfo
	Reason        RoomCleanupReason
	LastMessageAt time.Time
}

type RoomCleanupCandidateOptions struct {
	// Filtering rules
	AbandonedBefore     time.Time
	FilterOnlyForUserID id.UserID

	// NoCacheCleanup disables candidate eviction: candidates are written to cache
	// and DeleteCandidateEntries is not called. Use for analytics runs before real deletion.
	NoCacheCleanup bool

	//
	Workers       int
	OnRoomChecked func(ctx context.Context, roomInfo synapseadmin.RoomInfo)

	// OnRoomError is called when a single room can't be checked. Return true to
	// skip the failed room and continue iterating.
	OnRoomError func(ctx context.Context, roomInfo synapseadmin.RoomInfo, err error) bool
}

type RoomCleanupIterator struct {
	client            *Client
	roomActivityCache RoomActivityCache
}

func NewRoomCleanupIterator(client *Client, cache RoomActivityCache) *RoomCleanupIterator {
	return &RoomCleanupIterator{client: client, roomActivityCache: cache}
}

func (it *RoomCleanupIterator) lastRoomMessageAt(ctx context.Context, roomID id.RoomID) (time.Time, error) {
	now := time.Now()
	tsToEventResp, err := it.client.AdminTimestampToEvent(ctx, roomID, now, mautrix.DirectionBackward)
	if err != nil {
		return time.Time{}, fmt.Errorf("timestamp to event: %w", err)
	}

	ctxResp, err := it.client.AdminContext(ctx, roomID, tsToEventResp.EventID, nil, 0)
	if err != nil {
		return time.Time{}, fmt.Errorf("admin event context: %w", err)
	}

	filters := &mautrix.FilterPart{
		Types: []event.Type{
			event.EventRedaction, event.EventMessage,
			event.EventEncrypted, event.EventReaction,
		},
	}
	messageResp, err := it.client.RoomMessages(ctx, roomID, ctxResp.End, "", mautrix.DirectionBackward, filters, 1)
	if err != nil {
		return time.Time{}, fmt.Errorf("room messages: %w", err)
	}

	if len(messageResp.Chunk) == 0 {
		return time.Time{}, nil
	}

	return time.UnixMilli(messageResp.Chunk[0].Timestamp), nil
}

func (it *RoomCleanupIterator) candidate(
	ctx context.Context,
	roomInfo synapseadmin.RoomInfo,
	opts RoomCleanupCandidateOptions,
) (*RoomCleanupCandidate, error) {
	if roomInfo.JoinedMembers <= 0 {
		if opts.NoCacheCleanup {
			_ = it.roomActivityCache.StoreRoomActivity(ctx, RoomActivityCacheEntry{
				RoomID:        roomInfo.RoomID,
				LastMessageAt: time.Time{},
				JoinedMembers: roomInfo.JoinedMembers,
			})
		}

		return &RoomCleanupCandidate{
			Room:   roomInfo,
			Reason: RoomCleanupReasonEmpty,
		}, nil
	}

	entry, err := it.roomActivityCache.RoomActivity(ctx, roomInfo.RoomID)
	if err == nil && entry != nil {
		if entry.LastMessageAt.IsZero() {
			return &RoomCleanupCandidate{
				Room:   roomInfo,
				Reason: RoomCleanupReasonNoMessages,
			}, nil
		}

		if !entry.LastMessageAt.Before(opts.AbandonedBefore) {
			return nil, nil
		}
	}

	lastMessageAt, err := it.lastRoomMessageAt(ctx, roomInfo.RoomID)
	if err != nil {
		return nil, err
	}

	if lastMessageAt.IsZero() {
		if opts.NoCacheCleanup {
			_ = it.roomActivityCache.StoreRoomActivity(ctx, RoomActivityCacheEntry{
				RoomID:        roomInfo.RoomID,
				LastMessageAt: lastMessageAt,
				JoinedMembers: roomInfo.JoinedMembers,
			})
		}

		return &RoomCleanupCandidate{
			Room:   roomInfo,
			Reason: RoomCleanupReasonNoMessages,
		}, nil
	}

	if lastMessageAt.Before(opts.AbandonedBefore) {
		if opts.NoCacheCleanup {
			_ = it.roomActivityCache.StoreRoomActivity(ctx, RoomActivityCacheEntry{
				RoomID:        roomInfo.RoomID,
				LastMessageAt: lastMessageAt,
				JoinedMembers: roomInfo.JoinedMembers,
			})
		}

		return &RoomCleanupCandidate{
			Room:          roomInfo,
			Reason:        RoomCleanupReasonAbandoned,
			LastMessageAt: lastMessageAt,
		}, nil
	}

	_ = it.roomActivityCache.StoreRoomActivity(ctx, RoomActivityCacheEntry{
		RoomID:        roomInfo.RoomID,
		LastMessageAt: lastMessageAt,
		JoinedMembers: roomInfo.JoinedMembers,
	})

	return nil, nil
}

func (it *RoomCleanupIterator) workerFn(
	ctx context.Context,
	opts RoomCleanupCandidateOptions,
	jobs chan synapseadmin.RoomInfo,
	candidates chan RoomCleanupCandidate,
) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case roomInfo, ok := <-jobs:
			if !ok {
				return nil
			}
			if opts.OnRoomChecked != nil {
				opts.OnRoomChecked(ctx, roomInfo)
			}

			candidate, err := it.candidate(ctx, roomInfo, opts)
			if err != nil {
				if opts.OnRoomError != nil && opts.OnRoomError(ctx, roomInfo, err) {
					continue
				}

				return fmt.Errorf("check room %s: %w", roomInfo.RoomID, err)
			}
			if candidate == nil {
				continue
			}

			select {
			case <-ctx.Done():
				return nil
			case candidates <- *candidate:
			}
		}
	}
}

func (it *RoomCleanupIterator) emitRoomsGlobal(ctx context.Context, jobs chan synapseadmin.RoomInfo) error {
	listReq := synapseadmin.ReqListRoom{
		Direction: mautrix.DirectionBackward,
		OrderBy:   "joined_members",
		Limit:     1000,
	}

	return it.client.ListRoomsIt(ctx, listReq, func(ctx context.Context, roomInfo synapseadmin.RoomInfo) bool {
		select {
		case <-ctx.Done():
			return false
		case jobs <- roomInfo:
			return true
		}
	})
}

func (it *RoomCleanupIterator) emitRoomsForUser(ctx context.Context, userID id.UserID, jobs chan synapseadmin.RoomInfo) error {
	resp, err := it.client.AdminUserJoinedRooms(ctx, userID)
	if err != nil {
		return fmt.Errorf("admin user joined rooms: %w", err)
	}

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(4)

	for _, roomID := range resp.JoinedRooms {
		g.Go(func() error {
			roomInfo, err := it.client.RoomInfo(gCtx, roomID)
			if err != nil {
				return fmt.Errorf("room info for %s: %w", roomID, err)
			}

			select {
			case jobs <- roomInfo:
			case <-gCtx.Done():
				return gCtx.Err()
			}

			return nil
		})
	}

	return g.Wait()
}

func (it *RoomCleanupIterator) Iterate(
	ctx context.Context,
	opts RoomCleanupCandidateOptions,
	yield func(ctx context.Context, candidate RoomCleanupCandidate) bool,
) error {
	workers := opts.Workers
	if workers <= 0 {
		workers = 1
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errG, ctx := errgroup.WithContext(ctx)
	jobs := make(chan synapseadmin.RoomInfo)
	candidates := make(chan RoomCleanupCandidate)

	for range workers {
		errG.Go(func() error {
			return it.workerFn(ctx, opts, jobs, candidates)
		})
	}

	errG.Go(func() error {
		defer close(jobs)

		if opts.FilterOnlyForUserID != "" {
			return it.emitRoomsForUser(ctx, opts.FilterOnlyForUserID, jobs)
		}

		return it.emitRoomsGlobal(ctx, jobs)
	})

	waitErrCh := make(chan error, 1)
	go func() {
		waitErrCh <- errG.Wait()
		close(candidates)
	}()

	stopped := false
	for candidate := range candidates {
		if stopped {
			continue
		}

		if candidate.Room.RoomType != event.RoomTypeDefault {
			continue
		}

		if !yield(ctx, candidate) {
			stopped = true
			cancel()
		}
	}

	err := <-waitErrCh
	if stopped && errors.Is(err, context.Canceled) {
		err = nil
	}

	if err == nil && !opts.NoCacheCleanup {
		_ = it.roomActivityCache.DeleteCandidateEntries(ctx, opts.AbandonedBefore)
	}

	return err
}
