# Architecture

A daemon-centric design (Approach A). One background process owns the single
BlueZ scan and the authoritative status; every frontend is a thin client.

## Why daemon-centric

BlueZ does not like multiple concurrent discovery sessions on one adapter, and
we want a tray, a GUI, a status-bar command, and notifications to coexist. A
single daemon means:

- **One** BLE scanner → no adapter contention.
- **One** source of truth for "current status" → all frontends agree.
- Frontends become trivial (read JSON, render) and independently
  add/remove-able.
- A natural fit for i3/bars: the bar runs a cheap `openpods status --waybar`
  that just reads the daemon's last-known status.

Rejected alternatives: a shared library with per-frontend scans (adapter
contention, duplicated state) and exposing our own D-Bus service (more
machinery than a local single-producer/few-consumer problem needs; a Unix
socket is easier to script from a bar). See [`README.md`](./README.md) for the
full comparison.

## Component diagram

```
                          ┌──────────────────────────────┐
                          │           openpodsd           │
                          │          (daemon)             │
   BlueZ (system bus) ───▶│  ┌────────┐    ┌───────────┐  │
   org.bluez D-Bus        │  │  ble   │───▶│  core/    │  │
   ManufacturerData+RSSI  │  │ scanner│    │  state    │  │
                          │  └────────┘    └─────┬─────┘  │
                          │   uses pods (decode) │        │
                          │                ┌─────▼──────┐ │
                          │                │ unix socket│ │  $XDG_RUNTIME_DIR/openpods.sock
                          │                │  (NDJSON)  │ │
                          │                └─────┬──────┘ │
                          │       notifications  │        │
                          └───────────│──────────│────────┘
            org.freedesktop.Notifications        │
                                                 │  (subscribe / one-shot read)
                 ┌───────────────┬───────────────┼───────────────┐
                 ▼               ▼               ▼                ▼
            CLI (status)   tray (systray)   GUI (Fyne)     bar (--waybar)
```

## Packages (Go module `openpods-linux`)

### `pods` — pure decode core (no I/O)
Direct port of `PodsStatus` + `Pod` + the model hierarchy. Input: the 27-byte
manufacturer payload (or its hex string). Output: a `Status` value.

```go
type Pod struct {
    Level     int  // 0..10, or 15 = disconnected
    Charging  bool
    InEar     bool
}
func (p Pod) Connected() bool   // Level <= 10
func (p Pod) Percent() (int, bool) // 10 -> 100; 0..9 -> level*10+5; (0,false) if disconnected
func (p Pod) Low() bool         // Level <= 1

type Model int // AirPods1, AirPodsPro, ..., BeatsStudio3, Unknown
type Status struct {
    Model   Model
    Single  bool
    Left, Right, Case Pod   // for Single models, the figure is in a single Pod
    Flipped bool
    Time    time.Time       // capture time (for staleness)
}

func Decode(payload []byte) (Status, error) // validates per beacon-protocol.md §1
```

No Bluetooth, no D-Bus, no UI imports. Fully unit-testable. This is where the
[`beacon-protocol.md`](./beacon-protocol.md) constants live.

### `ble` — BlueZ scanner
Wraps the system-bus conversation with `org.bluez` (via
`github.com/godbus/dbus/v5`). Responsibilities:

- Find/track an adapter; honor its `Powered` state.
- `SetDiscoveryFilter({Transport:"le", DuplicateData:true})` + `StartDiscovery`.
- Subscribe to `InterfacesAdded` and `PropertiesChanged`; pull
  `ManufacturerData[0x004C]` and `RSSI` from each `org.bluez.Device1`.
- Apply the **strongest-recent-beacon** heuristic and the **RSSI ≥ −60 dBm**
  gate (ported from `PodsStatusScanCallback`).
- Track AirPods **connection state** (`Device1.Connected` + service-UUID match)
  as an extra "are these mine / are they actually connected" signal — a
  Linux-only improvement over Android. See
  [`linux-bluetooth.md`](./linux-bluetooth.md).
- Emit decoded `pods.Status` values on a channel.

The D-Bus surface is hidden behind a small interface so tests can feed recorded
advertisement maps without hardware.

```go
type Scanner interface {
    Updates() <-chan pods.Status   // decoded, filtered beacons
    Connected() <-chan bool        // AirPods audio link up/down
    Close() error
}
```

### `core` — state + lifecycle
Holds the latest `pods.Status` and a `connected` flag (mirrors Android's
`mMaybeConnected`). Applies the **30 s freshness** rule: if no beacon for 30 s,
battery is reported as `stale` (figures hidden, "updating" shown) while the link
may still be connected. Decides when to fire notifications (connect / disconnect
/ optional low-battery edge transitions).

### `ipc` — Unix-socket server/client
- Server (in daemon): listens on `$XDG_RUNTIME_DIR/openpods.sock`. On connect it
  writes the current status immediately, then streams **newline-delimited JSON**
  on every change. Read-only; no commands needed for v1.
- Client (in CLI/tray/GUI): connect, read one line (one-shot) or keep reading
  (watch).

```json
{"connected":true,"stale":false,"model":"airpodspro","single":false,
 "left":{"percent":85,"charging":false,"in_ear":true},
 "right":{"percent":90,"charging":false,"in_ear":true},
 "case":{"percent":55,"charging":true},"updated":"2026-06-01T12:00:00Z"}
```

### Frontends

| Frontend | Package | Notes |
| --- | --- | --- |
| **CLI** | `cmd/openpods` | `status` (one-shot), `--json`, `--waybar`, `--watch`. Falls back to a standalone short scan if no daemon socket is present. Primary display on i3. |
| **Daemon** | `cmd/openpodsd` | Runs `ble`+`core`+`ipc`+notifications. Shipped as a systemd **user** unit. |
| **Notifications** | `notify` | Calls `org.freedesktop.Notifications.Notify` via godbus on connect/disconnect and (optionally) low battery. |
| **Tray** | `cmd/openpods-tray` | `fyne.io/systray` SNI icon + menu (battery summary, "Open window", "Quit"). Reads the socket. See i3/SNI caveat in [`linux-bluetooth.md`](./linux-bluetooth.md). |
| **GUI** | `cmd/openpods-gui` | Fyne window replicating the Android home screen: pod/case images + battery bars. Reads the socket. |

### `assets`
Pod artwork ported from the Android `res/drawable` (`pod`, `podpro`, `podmax`,
`pod_case`, … plus `_disconnected` variants), embedded with `//go:embed` for the
tray and GUI.

## Data flow (happy path)

1. AirPods broadcast a beacon → BlueZ updates `Device1.ManufacturerData`/`RSSI`.
2. `ble` receives `PropertiesChanged`, extracts payload + RSSI, applies
   strongest-beacon + RSSI gate, calls `pods.Decode`, emits `Status`.
3. `core` stores it, resets the 30 s staleness timer, diffs against the previous
   state, and asks `notify` to fire on meaningful transitions.
4. `ipc` pushes the new JSON line to every connected frontend.
5. Tray/GUI re-render; a `--watch` bar updates its line; a one-shot CLI prints
   and exits.

## Lifecycle

- **Daemon**: `openpodsd.service` (user unit, `WantedBy=default.target`,
  `Restart=on-failure`). Starts at login. The tray/GUI can also `systemctl
  --user start` it on demand and socket-activate is a possible later refinement.
- **Tray/GUI**: optional XDG autostart `.desktop` entries; both spawn the daemon
  if its socket is missing.
- **CLI**: no daemon required — does a bounded one-shot scan if the socket is
  absent (so `openpods status` works on a fresh install before enabling the
  service).

## Error handling

| Condition | Behavior |
| --- | --- |
| `org.bluez` not on the bus | Daemon waits for the name to appear (D-Bus `NameOwnerChanged`); retries with backoff. CLI prints a clear message, exits non-zero. |
| No adapter / adapter removed | Daemon idles, watches `InterfacesAdded` for an adapter; status = disconnected. |
| Bluetooth powered off | Watch adapter `Powered`; stop scan, report disconnected; auto-resume when powered on (mirrors Android BT on/off receivers). |
| D-Bus connection dropped | Reconnect with exponential backoff; re-establish discovery + subscriptions. |
| Malformed / short payload | Skipped silently (fails validation in `pods.Decode`). |
| No beacon ≥ 30 s | `core` marks status `stale`; frontends hide figures / show "updating"; link may remain "connected". |
| Permission / polkit denial on discovery | Surface a one-time actionable error (group/polkit hint); see [`linux-bluetooth.md`](./linux-bluetooth.md). |
| Frontend can't reach socket | Print/log a hint to start the daemon; CLI falls back to one-shot scan. |

Errors are logged (structured, `log/slog`) and never crash the daemon; the
design follows the Android app's "catch, log, keep running" posture.

## Testing

See [`roadmap.md`](./roadmap.md#testing-strategy). In short: pure table-driven
tests for `pods`; interface-mocked recorded advertisements for `ble`/`core`; a
`--replay` fake-beacon mode for developing frontends without AirPods; and a
manual integration checklist on the i3/X11 host with real hardware.
