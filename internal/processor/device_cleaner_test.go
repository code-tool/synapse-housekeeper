package processor

import (
	"context"
	"slices"
	"testing"
	"time"

	"go.uber.org/zap"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/synapseadmin"

	"synapse-housekeeper/internal/synapse"
)

type deletedDevice struct {
	userID   id.UserID
	deviceID id.DeviceID
}

type fakeDeviceCleanerClient struct {
	users   []synapseadmin.RespUserInfo
	devices map[id.UserID][]synapseadmin.DeviceInfo
	deleted []deletedDevice
}

func (f *fakeDeviceCleanerClient) ListUsers(ctx context.Context, req synapse.ReqListUsers) (*synapse.RespListUsers, error) {
	return &synapse.RespListUsers{Users: f.users, Total: len(f.users)}, nil
}

func (f *fakeDeviceCleanerClient) ListDevices(ctx context.Context, userID id.UserID) (*synapseadmin.RespListDevices, error) {
	return &synapseadmin.RespListDevices{Devices: f.devices[userID]}, nil
}

func (f *fakeDeviceCleanerClient) DeleteUserDevice(ctx context.Context, userID id.UserID, deviceID id.DeviceID) error {
	f.deleted = append(f.deleted, deletedDevice{userID: userID, deviceID: deviceID})
	return nil
}

func testDevice(deviceID id.DeviceID, displayName string, age time.Duration) synapseadmin.DeviceInfo {
	return synapseadmin.DeviceInfo{
		RespDeviceInfo: mautrix.RespDeviceInfo{
			DeviceID:    deviceID,
			DisplayName: displayName,
			LastSeenTS:  time.Now().Add(-age).UnixMilli(),
		},
	}
}

func staleDevices() []synapseadmin.DeviceInfo {
	return []synapseadmin.DeviceInfo{
		testDevice("device-1", "app", 100*24*time.Hour),
		testDevice("device-2", "app", 100*24*time.Hour),
		testDevice("device-3", "app", 100*24*time.Hour),
		testDevice("device-4", "app", 100*24*time.Hour),
		testDevice("device-5", "app", 100*24*time.Hour),
		testDevice("device-6", "app", 100*24*time.Hour),
	}
}

func TestDeviceCleanerProcessDevices(t *testing.T) {
	cleaner := NewDeviceCleaner(zap.NewNop(), nil)
	tests := []struct {
		name    string
		devices []synapseadmin.DeviceInfo
		want    []id.DeviceID
	}{
		{
			name: "skips small device sets",
			devices: []synapseadmin.DeviceInfo{
				testDevice("device-1", "app", 100*24*time.Hour),
				testDevice("device-2", "app", 100*24*time.Hour),
				testDevice("device-3", "app", 100*24*time.Hour),
				testDevice("device-4", "app", 100*24*time.Hour),
				testDevice("device-5", "app", 100*24*time.Hour),
			},
		},
		{
			name: "keeps newest two per display name",
			devices: []synapseadmin.DeviceInfo{
				testDevice("age-10", "app", 10*24*time.Hour),
				testDevice("age-15", "app", 15*24*time.Hour),
				testDevice("age-22", "app", 22*24*time.Hour),
				testDevice("age-30", "app", 30*24*time.Hour),
				testDevice("age-40", "app", 40*24*time.Hour),
				testDevice("age-50", "app", 50*24*time.Hour),
			},
			want: []id.DeviceID{"age-50", "age-40", "age-30", "age-22"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var yielded []id.DeviceID
			err := cleaner.processDevices(tt.devices, func(deviceID id.DeviceID, lastSeen time.Time) bool {
				yielded = append(yielded, deviceID)
				return true
			})
			if err != nil {
				t.Fatalf("processDevices() error = %v", err)
			}
			if !slices.Equal(yielded, tt.want) {
				t.Fatalf("yielded devices = %+v, want %+v", yielded, tt.want)
			}
		})
	}
}

func TestDeviceCleanerProcess(t *testing.T) {
	userID := id.UserID("@user:test")
	tests := []struct {
		name       string
		doRealJob  bool
		wantDelete int
	}{
		{
			name:      "dry run does not delete",
			doRealJob: false,
		},
		{
			name:       "deletes stale devices when enabled",
			doRealJob:  true,
			wantDelete: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeDeviceCleanerClient{
				users: []synapseadmin.RespUserInfo{{UserID: userID}},
				devices: map[id.UserID][]synapseadmin.DeviceInfo{
					userID: staleDevices(),
				},
			}

			err := NewDeviceCleaner(zap.NewNop(), client).Process(context.Background(), tt.doRealJob)
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}
			if len(client.deleted) != tt.wantDelete {
				t.Fatalf("deleted devices = %+v, want %d", client.deleted, tt.wantDelete)
			}
		})
	}
}
