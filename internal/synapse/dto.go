package synapse

import (
	"encoding/json"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type AdminEvent struct {
	AuthEvents     []id.EventID                   `json:"auth_events"`
	Content        json.RawMessage                `json:"content"`
	Depth          int64                          `json:"depth"`
	Hashes         *event.PolicyHashes            `json:"hashes,omitzero"`
	OriginServerTS int64                          `json:"origin_server_ts"`
	PrevEvents     []id.EventID                   `json:"prev_events"`
	Redacts        *id.EventID                    `json:"redacts,omitzero"`
	RoomID         id.RoomID                      `json:"room_id,omitzero"` // not present for room v12+ create events
	Sender         id.UserID                      `json:"sender"`
	Signatures     map[string]map[id.KeyID]string `json:"signatures,omitzero"`
	Type           string                         `json:"type"`
	Unsigned       json.RawMessage                `json:"unsigned,omitzero"`
}

type RespAdminFetchEvent struct {
	Event AdminEvent `json:"event"`
}
