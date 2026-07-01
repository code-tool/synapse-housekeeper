# synapse-housekeeper

A set of command-line tools for keeping a [Synapse](https://github.com/element-hq/synapse)
Matrix homeserver tidy: purging abandoned rooms, pruning stale devices, and
flagging service accounts as bots. It talks to Synapse through the
[admin API](https://element-hq.github.io/synapse/latest/usage/administration/admin_api/)
and optionally uses PostgreSQL to persist room activity and purge scheduling
between runs.

## Commands

| Command | Description |
| --- | --- |
| `cleanup-rooms` | Delete rooms that have no remaining users and no recent activity. |
| `cleanup-devices` | Delete stale devices belonging to users. |
| `mark-as-bot` | Set a user's account type to `bot`. |

Every destructive command runs as a **dry run by default**. Pass `--do-real-job`
to actually perform the deletion — without it, the tool only reports what it
*would* do.

## Installation

Requires Go 1.26+.

```sh
go build -o synapse-housekeeper main.go
```

Or build the container image:

```sh
docker build -t synapse-housekeeper .
```

## Configuration

Configuration can be supplied either as command-line flags or environment
variables. A flag such as `--synapse-homeserver-url` maps to the environment
variable `SYNAPSE_HOMESERVER_URL` (uppercase, dashes become underscores).
Variables are also loaded automatically from a `.env` file in the working
directory.

| Flag | Environment variable | Description |
| --- | --- | --- |
| `--synapse-homeserver-url` | `SYNAPSE_HOMESERVER_URL` | Synapse homeserver URL. |
| `--synapse-user-id` | `SYNAPSE_USER_ID` | Admin user ID (`@user:server.com`). |
| `--synapse-access-token` | `SYNAPSE_ACCESS_TOKEN` | Admin access token. |
| `--postgres-dsn` | `POSTGRES_DSN` | PostgreSQL DSN for the room activity cache and purge schedule. |

### Global flags

| Flag | Default | Description |
| --- | --- | --- |
| `--debug` | `false` | Enable debug mode. |
| `--log-level` | `1` | Log level. |

Example `.env`:

```sh
SYNAPSE_HOMESERVER_URL=https://matrix.example.com
SYNAPSE_USER_ID=@admin:example.com
SYNAPSE_ACCESS_TOKEN=syt_your_admin_token
POSTGRES_DSN=postgres://user:password@localhost/synapse_housekeeper
```

## Usage

### Clean up abandoned rooms

Rooms with no messages for `--abandoned-days` and no remaining users are
candidates for deletion. Candidates are first soft-deleted, then fully purged
only after `--purge-cooldown-days` have elapsed, giving empty rooms a chance to
settle before the data is removed for good.

```sh
# Dry run — report what would be cleaned up
synapse-housekeeper cleanup-rooms

# Actually delete
synapse-housekeeper cleanup-rooms --do-real-job
```

| Flag | Default | Description |
| --- | --- | --- |
| `--abandoned-days` | `458` | Rooms with no messages for this many days are cleanup candidates. |
| `--purge-cooldown-days` | `14` | Days to wait after soft-delete before fully purging a room. |
| `--filter-only-for-user-id` | | When set, only check rooms joined by this user ID. |
| `--workers-count` | `4` | Number of concurrent room cleanup workers. |
| `--no-cache-cleanup` | `false` | Write candidates to the cache and skip eviction (for analytics before real deletion). |
| `--max-duration` | `0` | Stop the run after this wall-clock budget (e.g. `30m`, `2h`); `0` means no limit. |
| `--do-real-job` | `false` | Perform deletions. Without it, nothing is deleted. |

> **Note:** The purge cooldown is only persistent when `--postgres-dsn` is set.
> Without it, an in-memory store is used and rooms are purged without a cooldown.

### Clean up stale devices

```sh
synapse-housekeeper cleanup-devices --do-real-job
```

Like `cleanup-rooms`, this command accepts `--max-duration` to cap how long the
run may take. Reaching the budget is a clean stop — because every operation is
idempotent, whatever was left is simply picked up on the next run.

### Mark a user as a bot

```sh
synapse-housekeeper mark-as-bot --user-id @sync-bot:example.com
```

## License

Released under the [MIT License](LICENSE).
