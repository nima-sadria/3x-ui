# Mieru Provider

Mieru is an obfuscated proxy that is managed through the external `mita` binary.
It is **not** an Xray inbound — it never appears in `bin/config.json` and its
lifecycle is completely isolated from Xray. A failure in `mita` does not stop
Xray, and vice-versa.

## Prerequisites

Install the Mieru server tool on the same host:

```sh
# Follow https://github.com/enfein/mieru for your distro
# After installation, verify:
mita --version
```

## Enable the feature

Set the environment variable before starting x-ui:

```sh
ENABLE_MIERU_PROVIDER=true ./x-ui
```

Or add it to the systemd environment file (e.g. `/etc/default/x-ui`):

```ini
ENABLE_MIERU_PROVIDER=true
```

When the flag is absent or set to anything other than `true`, **no** Mieru
routes are registered, no background job runs, and no `mita` commands are ever
executed.

## Optional environment variables

| Variable | Default | Description |
|---|---|---|
| `ENABLE_MIERU_PROVIDER` | `false` | Set to `true` to activate the provider |
| `XUI_MIERU_CONFIG_DIR` | `$XUI_DB_FOLDER/mieru` | Directory for generated mita config files |

## Database

Two tables are created automatically on startup (GORM `AutoMigrate`):

| Table | Description |
|---|---|
| `mieru_inbounds` | One row per Mieru server configuration (name, port ranges, MTU, log level) |
| `mieru_users` | Client credentials bound to an inbound (username, password, traffic counters, expiry) |

No manual SQL migration is required — adding or changing Go struct fields and
restarting the binary is sufficient.

## API endpoints

All endpoints are under `/panel/api/mieru/` and require normal panel
authentication (session cookie, Bearer token, or mTLS).

### Inbounds

| Method | Path | Description |
|---|---|---|
| GET | `/inbounds` | List all inbounds |
| GET | `/inbounds/:id` | Get one inbound |
| POST | `/inbounds/add` | Create inbound |
| POST | `/inbounds/update/:id` | Update inbound |
| POST | `/inbounds/del/:id` | Delete inbound (cascades to users) |
| POST | `/inbounds/setEnable/:id` | `{"enable": true\|false}` |
| POST | `/inbounds/apply/:id` | Generate config and run `mita apply` |
| POST | `/inbounds/applyAll` | Apply first enabled inbound; stop mita if none |

### Users

| Method | Path | Description |
|---|---|---|
| GET | `/users/:inboundId` | List users for an inbound |
| GET | `/user/:id` | Get one user |
| POST | `/users/add` | Create user (auto-generates password if blank) |
| POST | `/users/update/:id` | Update user |
| POST | `/users/del/:id` | Delete user |
| POST | `/users/setEnable/:id` | `{"enable": true\|false}` |
| GET | `/users/export/:inboundId/:userId` | Download Mieru client profile JSON |
| GET | `/users/exportText/:inboundId/:userId` | Plain-text client config |

The export endpoints accept an optional `?server=<host>` query parameter to
override the server address embedded in the client profile.

### Mita lifecycle

| Method | Path | Description |
|---|---|---|
| GET | `/status` | mita availability, running state, `mita status` output |
| POST | `/start` | Run `mita start` |
| POST | `/stop` | Run `mita stop` |

## Inbound JSON schema

```json
{
  "name":         "my-server",
  "enable":       true,
  "tcpPortRange": "34787-34790",
  "udpPortRange": "33177-33180",
  "mtu":          1400,
  "loggingLevel": "INFO"
}
```

At least one of `tcpPortRange` or `udpPortRange` must be set. Port ranges are
either a single port (`"9000"`) or a dash-separated range (`"9000-9003"`).

## User JSON schema

```json
{
  "inboundId":      1,
  "username":       "alice",
  "password":       "hunter2",
  "enable":         true,
  "trafficLimitGB": 10,
  "expiryTime":     0
}
```

`trafficLimitGB = 0` means unlimited. `expiryTime` is a Unix millisecond
timestamp; `0` means never expires.

## Config generation and rollback

When `/inbounds/apply/:id` is called:

1. The panel builds a `MitaConfig` JSON from the inbound and its **enabled** users.
2. It writes the JSON to an atomic temp file in `$XUI_MIERU_CONFIG_DIR`.
3. It backs up the previously applied config (`generated-mita-config.json.bak`).
4. It runs `mita apply config <path>` then attempts `mita reload`.
5. If `mita apply` fails, the backup is restored and re-applied automatically.

## Background job

The `MieruJob` runs every 30 seconds when the provider is enabled:

1. Calls `mita get users` to collect per-user traffic counters.
2. Adds deltas to `mieru_users.up` / `mieru_users.down`.
3. Disables users whose `expiryTime` has passed or whose total traffic exceeds
   `trafficLimitGB` (only when `mita` exposes per-user stats).
4. If any users were disabled, calls `ApplyAllEnabledInbounds` to push the
   updated config to mita.

## Client export

The exported profile JSON is compatible with the official Mieru desktop client
and Hiddify. Example:

```sh
curl -s -H "Authorization: Bearer <token>" \
  "https://panel:2053/panel/api/mieru/users/export/1/3?server=1.2.3.4" \
  -o mieru-client.json
```

## Safety

- If `mita` is not installed, every lifecycle call returns a descriptive error
  (`MitaNotFoundError`) — the panel continues to run normally.
- Mieru tables are always migrated regardless of the feature flag, so enabling
  or disabling it at runtime does not corrupt the database.
- Existing Xray inbounds and all other panel features are completely unaffected.

## Manual verification

```sh
# Check status
curl -s -X GET http://localhost:2053/panel/api/mieru/status \
  -H "Authorization: Bearer <token>" | jq .

# Create an inbound
curl -s -X POST http://localhost:2053/panel/api/mieru/inbounds/add \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"name":"main","tcpPortRange":"34787-34790","udpPortRange":"33177","mtu":1400,"loggingLevel":"INFO"}' | jq .

# Create a user (password is auto-generated when omitted)
curl -s -X POST http://localhost:2053/panel/api/mieru/users/add \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"inboundId":1,"username":"alice"}' | jq .

# Apply config to mita
curl -s -X POST http://localhost:2053/panel/api/mieru/inbounds/apply/1 \
  -H "Authorization: Bearer <token>" | jq .
```
