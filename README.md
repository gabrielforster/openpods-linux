# OpenPods for Linux

Monitor your AirPods (and compatible Beats) battery on a Linux desktop — a
faithful Go port of the Android [OpenPods](https://github.com/adolfintel/OpenPods)
app by Federico Dossena.

> **Status: in development.** The beacon decode core (`pods`), the BlueZ
> scanner (`ble`), and a one-shot `openpods status` CLI are implemented and
> tested. The daemon, IPC, notifications, tray, and GUI are not built yet — see
> the [roadmap](docs/roadmap.md). The repo also holds the design docs and a copy
> of the upstream Android implementation as a knowledge base.

## What it does

AirPods broadcast their battery state in Bluetooth Low Energy advertising
packets. OpenPods-Linux **passively listens** for those packets via BlueZ,
decodes them, and shows **left / right / case battery %, charging, and in-ear
status**. It does **not** manage the audio connection — your OS (BlueZ +
PipeWire/PulseAudio) already does that, exactly like on Android.

## Planned frontends

- **CLI / daemon** — `openpods status [--json|--waybar|--watch]`, ideal for
  i3/polybar/waybar status bars (the primary display on a tiling WM).
- **Desktop notifications** — on connect/disconnect via `libnotify`.
- **System-tray icon** — StatusNotifierItem (KDE / GNOME+ext / waybar).
- **GUI window** — Fyne, replicating the Android home screen.

A single background daemon (`openpodsd`) owns the BlueZ scan and serves status to
all frontends over a Unix socket. See [`docs/architecture.md`](docs/architecture.md).

## Documentation

| Doc | Contents |
| --- | --- |
| [`docs/README.md`](docs/README.md) | Design overview + scope decisions |
| [`docs/architecture.md`](docs/architecture.md) | Packages, data flow, IPC, lifecycle, errors, testing |
| [`docs/beacon-protocol.md`](docs/beacon-protocol.md) | Reverse-engineered Apple BLE beacon format |
| [`docs/linux-bluetooth.md`](docs/linux-bluetooth.md) | BlueZ specifics + per-desktop tray/notification caveats |
| [`docs/roadmap.md`](docs/roadmap.md) | Phased, MVP-first build plan + test strategy |
| [`docs/android-reference/`](docs/android-reference/) | Upstream Android source + artwork (ground truth) |

## Building and running

```sh
go build ./...
go test ./...
```

`openpods status` runs a bounded one-shot scan and prints the current battery:

```sh
go run ./cmd/openpods status              # scan real AirPods via BlueZ
go run ./cmd/openpods status --replay     # no hardware: canned demo data
go run ./cmd/openpods status --timeout 5s # bound the scan duration
```

Example output:

```
AirPods Pro
  Left      55%
  Right    100%
  Case      85%
```

Target toolchain (dev host): Go ≥ 1.25, BlueZ ≥ 5.72, Linux with `org.bluez` on
the system D-Bus.

The remaining frontends are being built incrementally following the design;
[`PROMPT.md`](PROMPT.md) is the kickoff prompt that points an AI coding agent at
the docs and the next phase.

## License

GPLv3 — see [`LICENSE`](LICENSE). This is a derivative of OpenPods (© 2019–2022
Federico Dossena). AirPods and Beats are trademarks of Apple Inc.
