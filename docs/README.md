# OpenPods for Linux — Design Docs

> Status: **Feature-complete** (Phases 0–4) — `pods` decode core, `ble` BlueZ
> scanner, the `openpodsd` daemon (`core` + `ipc` + `notify`), and all four
> frontends (`openpods` CLI, `openpods-tray`, `openpods-gui`) are implemented
> and tested. Remaining: verification with real hardware and on each desktop. ·
> Target language: **Go**

This folder contains the design for a Linux port of [OpenPods](https://github.com/adolfintel/OpenPods)
by **Federico Dossena** (© 2019–2022, GPLv3): a tool that shows the battery level
of your AirPods (and compatible Beats) on a Linux desktop, the same way the
Android app does.

## What this is (and is not)

**It is** a *passive battery monitor*. AirPods broadcast their battery state in
Bluetooth Low Energy (BLE) advertising packets. This tool listens for those
packets, decodes them, and shows left / right / case battery %, charging state,
and in-ear detection.

**It is not** a Bluetooth connection manager. On Linux your AirPods already pair
and stream audio through the OS Bluetooth stack (BlueZ + PipeWire/PulseAudio),
exactly like any other Bluetooth headset. This tool never touches the audio
link — it only *reads* the advertised status. This mirrors what the Android app
does: Android also relies on the OS for the audio connection and only listens to
the BLE beacons for battery.

## Scope (decided)

| Decision | Choice |
| --- | --- |
| Goal | Battery monitoring only — a faithful port of the Android app's functionality |
| Language / stack | Go (BlueZ over D-Bus; `fyne/systray`; `godbus` notifications; Fyne GUI) |
| Repository | This **separate** standalone repo (`openpods-linux`); the upstream Android app ([adolfintel/OpenPods](https://github.com/adolfintel/OpenPods)) lives in its own repo |
| Frontends | CLI/daemon, desktop notifications, system-tray icon, full GUI window |
| Architecture | **Daemon-centric** — one background process owns BlueZ; thin frontends read from it over a Unix socket |
| Desktop targets | Portable to freedesktop standards; primary dev/test host is **i3 on X11** |

## Why a separate repo

The decoding logic and pod artwork are shared *conceptually* with the Android
app, but the runtime, build system (Go modules vs Gradle), and lifecycle are
completely different. A separate repo gets independent CI, releases, and issue
tracking without bolting a Go module onto a Gradle/Android tree. The durable
shared asset — the reverse-engineered beacon format — is captured here in
[`beacon-protocol.md`](./beacon-protocol.md) so both projects can track it.

## How the Linux version maps to the Android app

| Android component | Linux equivalent |
| --- | --- |
| `BluetoothLeScanner` + `ScanFilter` (`PodsStatusScanCallback`) | BlueZ D-Bus discovery + `ManufacturerData`/`RSSI` (`ble` package) |
| Beacon decode (`PodsStatus`, `Pod`, `models/*`) | Pure `pods` package — direct port, no platform deps |
| Foreground `Service` + `NotificationThread` | `openpodsd` daemon (systemd **user** service) |
| Ongoing status notification (custom `RemoteViews`) | Tray icon + GUI window + on-event `libnotify` notifications |
| "Strongest recent beacon" MAC workaround | Same heuristic, plus a Linux-only connection-state gate |
| BT on/off + ACL connect/disconnect receivers | BlueZ `PropertiesChanged` on adapter `Powered` and device `Connected` |

## Document map

1. [`architecture.md`](./architecture.md) — package layout, data flow, IPC, lifecycle, error handling, testing.
2. [`beacon-protocol.md`](./beacon-protocol.md) — the reverse-engineered Apple BLE beacon format, byte by byte. The most valuable durable reference.
3. [`linux-bluetooth.md`](./linux-bluetooth.md) — how BLE scanning works on Linux via BlueZ, the "which AirPods are mine" problem, and per-desktop tray/notification caveats (especially i3/Wayland).
4. [`roadmap.md`](./roadmap.md) — phased, MVP-first build order and the testing strategy.
5. [`android-reference/`](./android-reference/) — verbatim copies of the upstream Android source (the decode + scan + notification code) and pod artwork, indexed by [`android-reference/KNOWLEDGE_BASE.md`](./android-reference/KNOWLEDGE_BASE.md). Ground truth for the port.

## Target environment (verified on the dev host)

- Go `1.25.5`
- BlueZ `5.72` (supports both classic D-Bus discovery and the `AdvertisementMonitor` API)
- `org.bluez` present and D-Bus-activatable
- Desktop: **i3** on **X11** — drives the emphasis on the CLI/status-bar frontend (see [`linux-bluetooth.md`](./linux-bluetooth.md) for why the SNI tray needs a bridge here).
