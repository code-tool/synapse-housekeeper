package processor

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/synapseadmin"

	"synapse-housekeeper/internal/synapse"
)

type fakeRoomCleanerClient struct {
	deleteCalls int
	deleteReqs  []synapseadmin.ReqDeleteRoom
	deleteErr   error

	statusCalls int
	statusResp  synapse.RespDeleteStatus
	statusErr   error
}

func (f *fakeRoomCleanerClient) DeleteRoom(ctx context.Context, roomID id.RoomID, req synapseadmin.ReqDeleteRoom) (synapseadmin.RespDeleteRoom, error) {
	f.deleteCalls++
	f.deleteReqs = append(f.deleteReqs, req)

	return synapseadmin.RespDeleteRoom{DeleteID: "delete-id"}, f.deleteErr
}

func (f *fakeRoomCleanerClient) DeleteStatus(ctx context.Context, roomID id.RoomID) (synapse.RespDeleteStatus, error) {
	f.statusCalls++

	return f.statusResp, f.statusErr
}

func TestRoomCleanerDeleteRoom(t *testing.T) {
	httpBadRequestErr := mautrix.HTTPError{Response: &http.Response{StatusCode: http.StatusBadRequest}}

	tests := []struct {
		name            string
		client          fakeRoomCleanerClient
		wantErr         bool
		wantStatusCalls int
	}{
		{name: "success"},
		{
			name: "suppresses already scheduled delete",
			client: fakeRoomCleanerClient{
				deleteErr: httpBadRequestErr,
				statusResp: synapse.RespDeleteStatus{
					Results: []synapse.DeleteStatus{{Status: "purging"}},
				},
			},
			wantStatusCalls: 1,
		},
		{
			name:            "returns ambiguous HTTP 400",
			client:          fakeRoomCleanerClient{deleteErr: httpBadRequestErr},
			wantErr:         true,
			wantStatusCalls: 1,
		},
		{
			name:    "returns delete errors",
			client:  fakeRoomCleanerClient{deleteErr: errors.New("delete failed")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.client
			cleaner := NewRoomCleaner(zap.NewNop(), &client, nil, NewRoomPurgeScheduleMemory(), 1)

			err := cleaner.deleteRoom(context.Background(), "!room:test", true)
			if (err != nil) != tt.wantErr {
				t.Fatalf("deleteRoom() error = %v, wantErr %v", err, tt.wantErr)
			}
			if client.deleteCalls != 1 {
				t.Fatalf("DeleteRoom calls = %d, want 1", client.deleteCalls)
			}
			if client.statusCalls != tt.wantStatusCalls {
				t.Fatalf("DeleteStatus calls = %d, want %d", client.statusCalls, tt.wantStatusCalls)
			}
		})
	}
}

func TestRoomCleanerPurgeRoom(t *testing.T) {
	const roomID = id.RoomID("!room:test")
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	cooldown := 7 * 24 * time.Hour

	tests := []struct {
		name           string
		doRealJob      bool
		joinedMembers  int
		seedRecord     bool
		seedPurgeAfter time.Time

		wantDeleteCalls      int
		wantPurge            bool
		wantRecord           bool
		wantRecordPurgeAfter time.Time
		wantSoftDeleted      int64
		wantCooldownSkip     int64
		wantPurged           int64
	}{
		{
			name:                 "members present and unscheduled soft-deletes",
			doRealJob:            true,
			joinedMembers:        2,
			wantDeleteCalls:      1,
			wantPurge:            false,
			wantRecord:           true,
			wantRecordPurgeAfter: now.Add(cooldown),
			wantSoftDeleted:      1,
		},
		{
			name:                 "members present but already scheduled skips",
			doRealJob:            true,
			joinedMembers:        2,
			seedRecord:           true,
			seedPurgeAfter:       now.Add(time.Hour),
			wantRecord:           true,
			wantRecordPurgeAfter: now.Add(time.Hour),
		},
		{
			name:                 "empty within cooldown skips",
			doRealJob:            true,
			joinedMembers:        0,
			seedRecord:           true,
			seedPurgeAfter:       now.Add(time.Hour),
			wantRecord:           true,
			wantRecordPurgeAfter: now.Add(time.Hour),
			wantCooldownSkip:     1,
		},
		{
			name:            "empty after cooldown purges and clears schedule",
			doRealJob:       true,
			joinedMembers:   0,
			seedRecord:      true,
			seedPurgeAfter:  now.Add(-time.Hour),
			wantDeleteCalls: 1,
			wantPurge:       true,
			wantPurged:      1,
		},
		{
			name:            "naturally empty purges immediately",
			doRealJob:       true,
			joinedMembers:   0,
			wantDeleteCalls: 1,
			wantPurge:       true,
			wantPurged:      1,
		},
		{
			name:          "dry run does nothing",
			doRealJob:     false,
			joinedMembers: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			client := &fakeRoomCleanerClient{}
			schedule := NewRoomPurgeScheduleMemory()
			if tt.seedRecord {
				if err := schedule.Schedule(ctx, roomID, tt.seedPurgeAfter); err != nil {
					t.Fatalf("seed Schedule() error = %v", err)
				}
			}
			cleaner := NewRoomCleaner(zap.NewNop(), client, nil, schedule, 1)
			cleaner.now = func() time.Time { return now }
			stat := &RoomCleanerStatistics{}
			room := &synapseadmin.RoomInfo{RoomID: roomID, JoinedMembers: tt.joinedMembers}

			if err := cleaner.purgeRoom(ctx, tt.doRealJob, cooldown, stat, room); err != nil {
				t.Fatalf("purgeRoom() error = %v", err)
			}

			if client.deleteCalls != tt.wantDeleteCalls {
				t.Fatalf("DeleteRoom calls = %d, want %d", client.deleteCalls, tt.wantDeleteCalls)
			}
			if len(client.deleteReqs) > 0 && client.deleteReqs[0].Purge != tt.wantPurge {
				t.Fatalf("DeleteRoom purge = %v, want %v", client.deleteReqs[0].Purge, tt.wantPurge)
			}

			rec, err := schedule.Get(ctx, roomID)
			if err != nil {
				t.Fatalf("schedule Get() error = %v", err)
			}
			if tt.wantRecord {
				if rec == nil {
					t.Fatalf("schedule record = nil, want PurgeAfter %v", tt.wantRecordPurgeAfter)
				}
				if !rec.PurgeAfter.Equal(tt.wantRecordPurgeAfter) {
					t.Fatalf("schedule PurgeAfter = %v, want %v", rec.PurgeAfter, tt.wantRecordPurgeAfter)
				}
			} else if rec != nil {
				t.Fatalf("schedule record = %v, want none", rec)
			}

			if stat.SoftDeleted != tt.wantSoftDeleted {
				t.Fatalf("SoftDeleted = %d, want %d", stat.SoftDeleted, tt.wantSoftDeleted)
			}
			if stat.CooldownSkipped != tt.wantCooldownSkip {
				t.Fatalf("CooldownSkipped = %d, want %d", stat.CooldownSkipped, tt.wantCooldownSkip)
			}
			if stat.Purged != tt.wantPurged {
				t.Fatalf("Purged = %d, want %d", stat.Purged, tt.wantPurged)
			}
		})
	}
}
