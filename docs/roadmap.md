# Roadmap & Testing Strategy

MVP-first. Each phase is independently useful and ends with something runnable on
the i3/X11 dev host. All four requested frontends are delivered, ordered so the
highest-value, lowest-risk work lands first.

## Phase 0 — Decode core (`pods`)
**Goal:** a verified, pure Go port of the beacon decoder. Zero platform risk.

- Port `PodsStatus` / `Pod` / model detection per
  [`beacon-protocol.md`](./beacon-protocol.md), with positions/constants named.
- `Decode([]byte) (Status, error)` with full validation.
- Table-driven unit tests (see Testing Strategy) — must pass before anything
  else is built.

**Done when:** `go test ./pods/...` is green with vectors covering every model,
the flipped case, charging, and disconnected (`15`) nibbles.

## Phase 1 — BlueZ scanner + standalone CLI (`ble`, `cmd/openpods`)
**Goal:** prove the whole concept end-to-end on real hardware.

- `ble` scanner over `godbus/dbus/v5`: discovery filter, `PropertiesChanged`
  subscription, `ManufacturerData[0x004C]` + `RSSI` extraction.
- Strongest-recent-beacon + `RSSI ≥ -60` heuristic; connection-state gate.
- `openpods status` does a bounded one-shot scan and prints human-readable text.

**Done when:** with AirPods out of the case, `openpods status` prints correct
L/R/case battery on the i3 box.

## Phase 2 — Daemon, IPC, bar output, notifications
**Goal:** genuinely useful day-to-day on i3.

- `core` state + 30 s staleness rule + connect/disconnect/low-battery edges.
- `openpodsd` daemon; `ipc` Unix-socket NDJSON server; systemd **user** unit.
- `openpods status` learns `--json`, `--waybar`, `--watch`; falls back to
  one-shot scan when no daemon.
- `notify` package → `org.freedesktop.Notifications` on connect/disconnect.

**Done when:** daemon runs as a user service; an i3blocks/polybar module shows
live battery; a notification fires on connect; battery hides after 30 s idle.

## Phase 3 — Tray icon (`cmd/openpods-tray`)
**Goal:** first-class display on KDE/GNOME(+ext)/waybar.

- `fyne/systray` SNI icon + menu (summary, open window, quit), reading the socket.
- Battery-aware icon; document the i3/polybar **XEmbed vs SNI** caveat and the
  `snixembed` bridge (see [`linux-bluetooth.md`](./linux-bluetooth.md)).

**Done when:** tray icon shows status on a SNI desktop; docs explain the i3 path.

## Phase 4 — GUI window (`cmd/openpods-gui`)
**Goal:** the detailed "home screen" view, portable everywhere.

- Fyne window replicating the Android layout: pod/case images + battery bars +
  in-ear/charging indicators, reading the socket and live-updating.
- Embed ported artwork from Android `res/drawable`.

**Done when:** window renders the correct model image and live battery, launchable
from CLI/tray.

## Later / optional (explicitly out of v1 scope)
- `AdvertisementMonitor` API path (power-efficient passive scan) with auto-detect.
- Packaging: AUR package, Flatpak, prebuilt release binaries + `go install`.
- Config file (RSSI threshold, poll interval, which transitions notify).
- Low-battery threshold notifications as a configurable feature.
- D-Bus service surface in addition to the Unix socket, if a consumer wants it.

## Testing strategy

### `pods` — pure unit tests (no hardware)
Table-driven `(hexstring → expected Status)` rows. Mandatory coverage:
- each model ID from the §6 table,
- a flipped sample (L/R, charge bits, in-ear bits all swap),
- charging combinations,
- a `15` (disconnected) nibble and an out-of-range (`11`–`14`) nibble,
- malformed inputs (short, wrong prefix) → `Decode` returns an error.
Vectors captured via `btmon`/`busctl` per
[`beacon-protocol.md`](./beacon-protocol.md#8-reference-test-vectors).

### `ble` / `core` — interface-mocked, recorded advertisements
The D-Bus surface sits behind an interface. Tests feed recorded
`{ManufacturerData, RSSI, Connected}` sequences to verify:
- strongest-recent selection within the 10 s window,
- the −60 dBm gate,
- the 30 s staleness transition,
- connect/disconnect/low-battery edge detection (→ which notifications fire).
No real adapter required.

### `--replay` fake-beacon mode
A dev flag that feeds canned beacons into the pipeline so the tray/GUI/bar can be
built and demoed without AirPods present. Also handy for screenshots and CI smoke
tests of the frontends.

### Manual integration checklist (real hardware, i3/X11 host)
1. `bluetoothctl` shows AirPods connected as an audio device.
2. `openpods status` (no daemon) prints correct battery — Phase 1.
3. Enable `openpodsd` user service; bar module updates live — Phase 2.
4. Open/close the case, take a pod out → notifications + in-ear update.
5. Walk away (no beacon) → battery hides after ~30 s, returns on approach.
6. Tray on a SNI desktop (or via `snixembed` on i3); GUI window — Phases 3–4.

### CI
- `go vet`, `go test ./...`, `golangci-lint` on every push.
- Frontends smoke-tested headlessly via `--replay` where feasible (GUI tests are
  best-effort given display requirements).
