package processor

import (
	"context"
	"errors"
	"net/http"
	"testing"

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

func TestRoomCleanerPurgeRoom(t *testing.T) {
	httpBadRequestErr := mautrix.HTTPError{Response: &http.Response{StatusCode: http.StatusBadRequest}}

	tests := []struct {
		name            string
		doRealJob       bool
		client          fakeRoomCleanerClient
		wantErr         bool
		wantDeleteCalls int
		wantStatusCalls int
		wantPurge       bool
	}{
		{
			name:      "dry run does not delete",
			doRealJob: false,
		},
		{
			name:            "deletes with purge",
			doRealJob:       true,
			wantDeleteCalls: 1,
			wantPurge:       true,
		},
		{
			name:      "suppresses already scheduled delete",
			doRealJob: true,
			client: fakeRoomCleanerClient{
				deleteErr: httpBadRequestErr,
				statusResp: synapse.RespDeleteStatus{
					Results: []synapse.DeleteStatus{{Status: "purging"}},
				},
			},
			wantDeleteCalls: 1,
			wantStatusCalls: 1,
			wantPurge:       true,
		},
		{
			name:      "returns ambiguous HTTP 400",
			doRealJob: true,
			client: fakeRoomCleanerClient{
				deleteErr:  httpBadRequestErr,
				statusResp: synapse.RespDeleteStatus{},
			},
			wantErr:         true,
			wantDeleteCalls: 1,
			wantStatusCalls: 1,
			wantPurge:       true,
		},
		{
			name:      "returns delete errors",
			doRealJob: true,
			client: fakeRoomCleanerClient{
				deleteErr: errors.New("delete failed"),
			},
			wantErr:         true,
			wantDeleteCalls: 1,
			wantPurge:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.client
			cleaner := NewRoomCleaner(zap.NewNop(), &client, nil, 1)
			room := &synapseadmin.RoomInfo{RoomID: "!room:test", JoinedMembers: 1}

			err := cleaner.purgeRoom(context.Background(), tt.doRealJob, room)
			if (err != nil) != tt.wantErr {
				t.Fatalf("purgeRoom() error = %v, wantErr %v", err, tt.wantErr)
			}
			if client.deleteCalls != tt.wantDeleteCalls {
				t.Fatalf("DeleteRoom calls = %d, want %d", client.deleteCalls, tt.wantDeleteCalls)
			}
			if client.statusCalls != tt.wantStatusCalls {
				t.Fatalf("DeleteStatus calls = %d, want %d", client.statusCalls, tt.wantStatusCalls)
			}
			if len(client.deleteReqs) > 0 && client.deleteReqs[0].Purge != tt.wantPurge {
				t.Fatalf("DeleteRoom request purge = %v, want %v", client.deleteReqs[0].Purge, tt.wantPurge)
			}
		})
	}
}
