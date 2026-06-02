# Initial Build Prompt — OpenPods for Linux

Paste the prompt below into a fresh coding-agent session **with this repository as
the working directory**. It is self-contained: it points at the design docs and
the Android knowledge base already in this repo and starts the work at Phase 0.

Everything the agent needs is in `docs/`. The single source of truth for decoding
is `docs/beacon-protocol.md`; the upstream Android code in
`docs/android-reference/` is ground truth when prose and code disagree.

---

## PROMPT

You are implementing **OpenPods for Linux**, a Go port of the Android OpenPods app
that passively monitors AirPods/Beats battery over Bluetooth LE. The full design
already exists in this repo — read it before writing any code.

### Read first (in this order)
1. `docs/README.md` — scope and the decisions already made (do not relitigate them).
2. `docs/architecture.md` — the daemon-centric design and package layout you must follow.
3. `docs/beacon-protocol.md` — the exact byte/nibble format to decode. **Authoritative.**
4. `docs/linux-bluetooth.md` — how to scan via BlueZ/D-Bus and the desktop caveats.
5. `docs/roadmap.md` — the phased plan and the testing strategy.
6. `docs/android-reference/KNOWLEDGE_BASE.md` — index of the original Android source (ground truth).

### Fixed decisions (already made — honor them)
- **Goal:** battery monitoring only. Do **not** manage the audio connection.
- **Language:** Go (module `openpods-linux`).
- **Architecture:** daemon-centric (Approach A). One `openpodsd` owns the single
  BlueZ scan and serves status to thin frontends over a Unix socket (NDJSON at
  `$XDG_RUNTIME_DIR/openpods.sock`).
- **Frontends:** CLI/daemon, libnotify notifications, `fyne/systray` tray, Fyne GUI.
- **BLE:** talk to BlueZ over D-Bus with `github.com/godbus/dbus/v5`. Use plain
  discovery + `PropertiesChanged` for v1; `AdvertisementMonitor` is a later option.
- **Primary host:** i3 on X11 → the CLI/`--waybar` frontend is the headline display;
  the SNI tray needs `snixembed` here (document, don't fight it).
- **License:** GPLv3, derivative of OpenPods (© Federico Dossena) — keep attribution.

### How to work
- **Test-driven.** For the `pods` package especially, write table-driven tests from
  `docs/beacon-protocol.md` (and capture real vectors with `btmon`/`busctl`) before
  or alongside the implementation. `go test ./...` must stay green.
- Keep packages small and single-purpose with clear interfaces, as laid out in
  `docs/architecture.md` (`pods`, `ble`, `core`, `ipc`, `notify`, `cmd/*`, `assets`).
- Hide the D-Bus surface behind an interface so `ble`/`core` are testable without
  hardware. Add a `--replay` fake-beacon mode for developing frontends offline.
- Log with `log/slog`; follow the upstream "catch, log, keep running" posture for
  the daemon. Handle BT-off / no-adapter / D-Bus-drop per the error-handling table
  in `docs/architecture.md`.
- Match Go conventions: `go vet` clean, `gofmt`, idiomatic errors.

### Start with Phase 0 — the `pods` decode core
This is pure logic with zero platform risk and unblocks everything else.

1. `go mod init openpods-linux` (Go ≥ 1.25).
2. Create the `pods` package: a `Decode([]byte) (Status, error)` that validates the
   payload (length 27, prefix `0x07 0x19`) and decodes battery/charge/in-ear/model
   exactly per `docs/beacon-protocol.md`. Port `Pod` (level 0–10/15, `level*10+5`%
   display, low ≤ 1) and the model table from `docs/android-reference/`.
3. Write table-driven tests covering: every model id, a flipped sample, charging
   combinations, a `15` (disconnected) nibble, an out-of-range nibble, and malformed
   inputs. Make `go test ./pods/...` pass.

Then stop and summarize what you built, confirm the tests, and propose moving to
**Phase 1** (the `ble` BlueZ scanner + a standalone `openpods status` CLI) per the
roadmap.

### Definition of done for this first session
- `go.mod` exists; `pods` package implemented; `go test ./...` green; a short note
  on coverage and any beacon-format ambiguities you resolved (cite the doc/code).

---

### Notes for the human running this
- If you have AirPods handy, capture a few real beacons up front so the agent has
  concrete test vectors — see `docs/beacon-protocol.md` §8.
- Build phases in order (0 → 4). Each ends with something runnable; review between
  phases rather than letting it run the whole roadmap unattended.
- The agent should never need the upstream Android repo — everything is in
  `docs/android-reference/`.
