package synapse

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/synapseadmin"
)

type Client struct {
	*synapseadmin.Client
	roomActivityCache RoomActivityCache
}

func NewClient(homeserverURL string, userID id.UserID, accessToken string) (*Client, error) {
	baseClient, err := mautrix.NewClient(homeserverURL, userID, accessToken)
	if err != nil {
		return nil, err
	}
	baseClient.Client = &http.Client{Timeout: 5 * time.Minute}

	return &Client{
		Client:            &synapseadmin.Client{Client: baseClient},
		roomActivityCache: RoomActivityCacheNull{},
	}, nil
}

func (cli *Client) WithRoomActivityCache(cache RoomActivityCache) *Client {
	cli.roomActivityCache = cache

	return cli
}

func (cli *Client) MakeFullRequest(ctx context.Context, params mautrix.FullRequest) ([]byte, error) {
	return cli.Client.Client.MakeFullRequest(ctx, params)
}

func (cli *Client) BuildURLWithQuery(urlPath mautrix.PrefixableURLPath, urlQuery map[string]string) string {
	return cli.Client.Client.BuildURLWithQuery(urlPath, urlQuery)
}

func (cli *Client) MakeRequest(ctx context.Context, method string, httpURL string, reqBody any, resBody any) ([]byte, error) {
	return cli.Client.Client.MakeRequest(ctx, method, httpURL, reqBody, resBody)
}

func (cli *Client) BuildURL(urlPath mautrix.PrefixableURLPath) string {
	return cli.Client.Client.BuildURL(urlPath)
}

type ReqListUsers struct {
	UserID string
	Name   string

	Guests      *bool
	Admins      *bool
	Deactivated *bool
	Locked      *bool
	//
	OrderBy   string
	Direction mautrix.Direction
	From      string
	Limit     int
}

func (req *ReqListUsers) BuildQuery() map[string]string {
	query := map[string]string{}

	if req.From != "" {
		query["from"] = req.From
	}
	//
	if req.UserID != "" {
		query["user_id"] = req.UserID
	}
	if req.Name != "" {
		query["name"] = req.Name
	}

	//
	if req.Guests != nil {
		query["guests"] = strconv.FormatBool(*req.Guests)
	}
	if req.Admins != nil {
		query["admins"] = strconv.FormatBool(*req.Admins)
	}
	if req.Deactivated != nil {
		query["deactivated"] = strconv.FormatBool(*req.Deactivated)
	}
	if req.Locked != nil {
		query["locked"] = strconv.FormatBool(*req.Locked)
	}
	//
	if req.OrderBy != "" {
		query["order_by"] = req.OrderBy
	}
	if req.Direction != 0 {
		query["dir"] = string(req.Direction)
	}
	if req.Limit != 0 {
		query["limit"] = strconv.Itoa(req.Limit)
	}
	return query
}

type RespListUsers struct {
	Users     []synapseadmin.RespUserInfo `json:"users"`
	Total     int                         `json:"total"`
	NextToken string                      `json:"next_token,omitempty"`
}

func (cli *Client) ListUsers(ctx context.Context, req ReqListUsers) (resp *RespListUsers, err error) {
	_, err = cli.MakeFullRequest(ctx, mautrix.FullRequest{
		Method: http.MethodGet,
		URL:    cli.BuildURLWithQuery(mautrix.SynapseAdminURLPath{"v2", "users"}, req.BuildQuery()),

		ResponseJSON: &resp,
	})
	return
}

// DeleteUserDevice Deletes the given device_id for a specific user_id, and invalidates any access token associated with it.
//
// https://element-hq.github.io/synapse/latest/admin_api/user_admin_api.html#delete-a-device
func (cli *Client) DeleteUserDevice(ctx context.Context, userID id.UserID, deviceID id.DeviceID) (err error) {
	_, err = cli.MakeRequest(ctx, http.MethodDelete, cli.BuildAdminURL("v2", "users", userID, "devices", deviceID), nil, nil)
	return
}

func (cli *Client) ListRoomsIt(ctx context.Context, req synapseadmin.ReqListRoom, yield func(ctx context.Context, rInfo synapseadmin.RoomInfo) bool) error {
	for {
		resp, err := cli.ListRooms(ctx, req)
		if err != nil {
			return err
		}
		for _, room := range resp.Rooms {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				if !yield(ctx, room) {
					return nil
				}
			}
		}

		if len(resp.Rooms) < req.Limit || resp.NextBatch <= 0 {
			break
		}

		req.From = resp.NextBatch
	}

	return nil
}

func (cli *Client) RoomInfo(ctx context.Context, roomID id.RoomID) (resp synapseadmin.RoomInfo, err error) {
	_, err = cli.MakeFullRequest(ctx, mautrix.FullRequest{
		Method:       http.MethodGet,
		URL:          cli.BuildURL(mautrix.SynapseAdminURLPath{"v1", "rooms", roomID}),
		ResponseJSON: &resp,
	})

	return
}

type DeleteStatus struct {
	DeleteID     string `json:"delete_id"`
	Status       string `json:"status"`
	ShutdownRoom bool   `json:"shutdown_room,omitempty"`
}

type RespDeleteStatus struct {
	Results []DeleteStatus `json:"results"`
}

func (cli *Client) DeleteStatus(ctx context.Context, roomID id.RoomID) (resp RespDeleteStatus, err error) {
	_, err = cli.MakeFullRequest(ctx, mautrix.FullRequest{
		Method: http.MethodGet,
		URL:    cli.BuildURLWithQuery(mautrix.SynapseAdminURLPath{"v2", "rooms", roomID, "delete_status"}, nil),

		ResponseJSON: &resp,
	})

	return
}

type RespDeleteForwardExtremities struct {
	Deleted int `json:"deleted"`
}

func (cli *Client) DeleteForwardExtremities(ctx context.Context, roomID id.RoomID) (resp RespDeleteForwardExtremities, err error) {
	_, err = cli.MakeFullRequest(ctx, mautrix.FullRequest{
		Method: http.MethodDelete,
		URL:    cli.BuildURLWithQuery(mautrix.SynapseAdminURLPath{"v1", "rooms", roomID, "forward_extremities"}, nil),

		ResponseJSON: &resp,
	})

	return
}

// AdminTimestampToEvent Admin TimestampToEvent finds the ID of the event closest to the given timestamp.
//
// See https://element-hq.github.io/synapse/latest/admin_api/rooms.html#room-timestamp-to-event-api
func (cli *Client) AdminTimestampToEvent(ctx context.Context, roomID id.RoomID, timestamp time.Time, dir mautrix.Direction) (resp *mautrix.RespTimestampToEvent, err error) {
	query := map[string]string{
		"ts":  strconv.FormatInt(timestamp.UnixMilli(), 10),
		"dir": string(dir),
	}
	urlPath := cli.BuildURLWithQuery(mautrix.SynapseAdminURLPath{"v1", "rooms", roomID, "timestamp_to_event"}, query)
	_, err = cli.MakeRequest(ctx, http.MethodGet, urlPath, nil, &resp)
	return
}

// AdminFetchEvent  The fetch event API allows admins to fetch an event regardless of their membership in the room it originated in.
//
// See https://element-hq.github.io/synapse/latest/admin_api/fetch_event.html
func (cli *Client) AdminFetchEvent(ctx context.Context, eventID id.EventID) (resp *RespAdminFetchEvent, err error) {
	_, err = cli.MakeFullRequest(ctx, mautrix.FullRequest{
		Method: http.MethodGet,
		URL:    cli.BuildURLWithQuery(mautrix.SynapseAdminURLPath{"v1", "fetch_event", eventID}, nil),

		ResponseJSON: &resp,
	})

	return nil, nil
}

func (cli *Client) AdminContext(ctx context.Context, roomID id.RoomID, eventID id.EventID, filter *mautrix.FilterPart, limit int) (resp *mautrix.RespContext, err error) {
	query := map[string]string{}
	if filter != nil {
		filterJSON, err := json.Marshal(filter)
		if err != nil {
			return nil, err
		}
		query["filter"] = string(filterJSON)
	}
	if limit != 0 {
		query["limit"] = strconv.Itoa(limit)
	}

	urlPath := cli.BuildURLWithQuery(mautrix.SynapseAdminURLPath{"v1", "rooms", roomID, "context", eventID}, query)
	_, err = cli.MakeRequest(ctx, http.MethodGet, urlPath, nil, &resp)
	return
}
