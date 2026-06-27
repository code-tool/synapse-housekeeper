package synapse

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/synapseadmin"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := NewClient(server.URL, id.UserID("@admin:test"), "token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	return client
}

func TestClientListUsersBuildsQuery(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want %s", r.Method, http.MethodGet)
		}
		if r.URL.Path != "/_synapse/admin/v2/users" {
			t.Fatalf("path = %s", r.URL.Path)
		}

		query := r.URL.Query()
		if query.Get("from") != "10" {
			t.Fatalf("from query = %q", query.Get("from"))
		}
		if query.Get("limit") != "25" {
			t.Fatalf("limit query = %q", query.Get("limit"))
		}
		if query.Get("guests") != "false" {
			t.Fatalf("guests query = %q", query.Get("guests"))
		}
		if query.Get("locked") != "false" {
			t.Fatalf("locked query = %q", query.Get("locked"))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"users":[{"name":"@user:test"}],"total":1,"next_token":"next"}`))
	})

	f := false
	resp, err := client.ListUsers(ctx, ReqListUsers{
		Guests: &f,
		Locked: &f,
		From:   "10",
		Limit:  25,
	})
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if resp.Total != 1 || resp.NextToken != "next" || len(resp.Users) != 1 {
		t.Fatalf("ListUsers() response = %+v", resp)
	}
	if resp.Users[0].UserID != "@user:test" {
		t.Fatalf("user ID = %s", resp.Users[0].UserID)
	}
}

func TestClientDeleteUserDeviceUsesAdminEndpoint(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %s, want %s", r.Method, http.MethodDelete)
		}
		if !strings.HasPrefix(r.URL.Path, "/_synapse/admin/v2/users/") {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if !strings.HasSuffix(r.URL.Path, "/devices/DEVICE") {
			t.Fatalf("path = %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	if err := client.DeleteUserDevice(ctx, id.UserID("@user:test"), id.DeviceID("DEVICE")); err != nil {
		t.Fatalf("DeleteUserDevice() error = %v", err)
	}
}

func TestClientDeleteRoomSendsPurgeExplicitly(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		purge     bool
		wantPurge bool
	}{
		{name: "purge false is sent, not omitted", purge: false, wantPurge: false},
		{name: "purge true is sent", purge: true, wantPurge: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodDelete {
					t.Fatalf("method = %s, want %s", r.Method, http.MethodDelete)
				}
				if !strings.HasPrefix(r.URL.Path, "/_synapse/admin/v2/rooms/") {
					t.Fatalf("path = %s", r.URL.Path)
				}

				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				raw, ok := body["purge"]
				if !ok {
					t.Fatalf("purge field missing from request body %v", body)
				}
				if string(raw) != strconv.FormatBool(tt.wantPurge) {
					t.Fatalf("purge = %s, want %v", raw, tt.wantPurge)
				}

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"delete_id":"delete-id"}`))
			})

			resp, err := client.DeleteRoom(ctx, id.RoomID("!room:test"), synapseadmin.ReqDeleteRoom{Purge: tt.purge})
			if err != nil {
				t.Fatalf("DeleteRoom() error = %v", err)
			}
			if resp.DeleteID != "delete-id" {
				t.Fatalf("DeleteID = %q, want %q", resp.DeleteID, "delete-id")
			}
		})
	}
}

func TestClientAdminContextEncodesFilter(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want %s", r.Method, http.MethodGet)
		}
		if !strings.HasPrefix(r.URL.Path, "/_synapse/admin/v1/rooms/") {
			t.Fatalf("path = %s", r.URL.Path)
		}

		var filter mautrix.FilterPart
		if err := json.Unmarshal([]byte(r.URL.Query().Get("filter")), &filter); err != nil {
			t.Fatalf("filter query is not JSON: %v", err)
		}
		if len(filter.Types) != 1 || filter.Types[0] != event.EventMessage {
			t.Fatalf("filter types = %+v", filter.Types)
		}
		if r.URL.Query().Get("limit") != "7" {
			t.Fatalf("limit query = %q", r.URL.Query().Get("limit"))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})

	filter := &mautrix.FilterPart{Types: []event.Type{event.EventMessage}}
	if _, err := client.AdminContext(ctx, id.RoomID("!room:test"), id.EventID("$event"), filter, 7); err != nil {
		t.Fatalf("AdminContext() error = %v", err)
	}
}
