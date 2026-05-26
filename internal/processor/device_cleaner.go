package processor

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"go.uber.org/zap"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/synapseadmin"

	"synapse-housekeeper/internal/synapse"
)

type DeviceCleaner struct {
	log *zap.Logger

	synapseClient deviceCleanerClient
}

type deviceCleanerClient interface {
	ListUsers(ctx context.Context, req synapse.ReqListUsers) (resp *synapse.RespListUsers, err error)
	ListDevices(ctx context.Context, userID id.UserID) (resp *synapseadmin.RespListDevices, err error)
	DeleteUserDevice(ctx context.Context, userID id.UserID, deviceID id.DeviceID) error
}

func NewDeviceCleaner(log *zap.Logger, synapseClient deviceCleanerClient) *DeviceCleaner {
	return &DeviceCleaner{log: log, synapseClient: synapseClient}
}

func (s *DeviceCleaner) insertIntoOrderedDec(ts []synapseadmin.DeviceInfo, t synapseadmin.DeviceInfo) []synapseadmin.DeviceInfo {
	i, _ := slices.BinarySearchFunc(ts, t, func(a, b synapseadmin.DeviceInfo) int {
		return int(b.LastSeenTS) - int(a.LastSeenTS)
	})

	return slices.Insert(ts, i, t)
}

func (s *DeviceCleaner) processDevices(devices []synapseadmin.DeviceInfo, yield func(id.DeviceID, time.Time) bool) error {
	if len(devices) <= 5 {
		return nil
	}

	groups := make(map[string][]synapseadmin.DeviceInfo)
	for _, device := range devices {
		lastSeen := time.UnixMilli(device.LastSeenTS)

		if time.Since(lastSeen) <= 90*time.Hour*24 {
			groups[device.DisplayName] = s.insertIntoOrderedDec(groups[device.DisplayName], device)

			continue
		}

		if !yield(device.DeviceID, lastSeen) {
			return errors.New("can't remove device")
		}
	}

	const minDevicesInGroup = 2
	for _, gDevices := range groups {
		if len(gDevices) <= minDevicesInGroup {
			continue
		}

		for _, device := range Reverse(gDevices[minDevicesInGroup:]) {
			lastSeen := time.UnixMilli(device.LastSeenTS)

			if time.Since(lastSeen) <= 21*time.Hour*24 {
				break
			}

			if !yield(device.DeviceID, lastSeen) {
				return errors.New("can't remove device")
			}
		}
	}

	return nil
}

func (s *DeviceCleaner) iterateDeleteList(ctx context.Context, yield func(id.UserID, id.DeviceID, time.Time) bool) error {
	var (
		err             error
		req             synapse.ReqListUsers
		userListResp    *synapse.RespListUsers
		devicesListResp *synapseadmin.RespListDevices
	)

	f := false
	req.Guests = &f
	// req.Deactivated = &f
	req.Locked = &f
	// req.Admins = &f

	for {
		userListResp, err = s.synapseClient.ListUsers(ctx, req)
		if err != nil {
			return fmt.Errorf("list users: %w", err)
		}

		for idx := range userListResp.Users {
			if userListResp.Users[idx].UserType != "" {
				continue
			}

			devicesListResp, err = s.synapseClient.ListDevices(ctx, userListResp.Users[idx].UserID)
			if err != nil {
				return fmt.Errorf("list devices for user %s: %w", userListResp.Users[idx].UserID.String(), err)
			}

			err = s.processDevices(devicesListResp.Devices, func(deviceID id.DeviceID, t time.Time) bool {
				return yield(userListResp.Users[idx].UserID, deviceID, t)
			})

			if err != nil {
				return err
			}
		}

		if userListResp.NextToken == "" {
			break
		}

		req.From = userListResp.NextToken
	}

	return nil
}

func (s *DeviceCleaner) Process(ctx context.Context, doRealJob bool) error {
	var err error
	var cnt int
	iErr := s.iterateDeleteList(ctx, func(userID id.UserID, deviceID id.DeviceID, lastSeen time.Time) bool {
		s.log.Debug("Deleting user device", zap.Stringer("user", userID), zap.Stringer("device", deviceID), zap.Time("last_seen", lastSeen))

		if !doRealJob {
			cnt++

			return true
		}

		if err = s.synapseClient.DeleteUserDevice(ctx, userID, deviceID); err != nil {
			err = fmt.Errorf("delete device %s for user %s: %w", deviceID.String(), userID.String(), err)
		}

		cnt++

		return err == nil
	})

	s.log.Info("Deleted devices", zap.Int("count", cnt))

	return errors.Join(iErr, err)
}
