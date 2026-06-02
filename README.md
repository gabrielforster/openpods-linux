# OpenPods for Linux

Monitor your AirPods (and compatible Beats) battery on a Linux desktop — a
faithful Go port of the Android [OpenPods](https://github.com/adolfintel/OpenPods)
app by Federico Dossena.

> **Status: in development.** The decode core (`pods`), the BlueZ scanner
> (`ble`), the `openpodsd` daemon (state + staleness, Unix-socket NDJSON IPC,
> connect/disconnect notifications), and the `openpods` CLI (one-shot or
> daemon-backed, `--json`/`--waybar`/`--watch`) are implemented and tested. The
> tray icon and GUI window are not built yet — see the [roadmap](docs/roadmap.md).
> The repo also holds the design docs and a copy of the upstream Android
> implementation as a knowledge base.

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

### Run as a daemon

`openpodsd` owns a single BlueZ scan and serves status to thin frontends over a
Unix socket (`$XDG_RUNTIME_DIR/openpods.sock`), firing desktop notifications on
connect/disconnect. The `openpods` CLI then reads the daemon instead of scanning
itself (falling back to a one-shot scan if the daemon isn't running).

```sh
go build -o ~/.local/bin/openpodsd ./cmd/openpodsd
go build -o ~/.local/bin/openpods  ./cmd/openpods

# install and enable the user service
install -Dm644 packaging/systemd/openpodsd.service ~/.config/systemd/user/openpodsd.service
systemctl --user daemon-reload
systemctl --user enable --now openpodsd
```

Status-bar integration (the daemon makes each refresh cheap — it just reads the
last-known status, no per-refresh scan):

```ini
# i3blocks: ~/.config/i3blocks/config
[openpods]
command=openpods status --waybar | jq -r '.text'
interval=5
```

```ini
# polybar
[module/openpods]
type = custom/script
exec = openpods status --waybar | jq -r .text
interval = 5
```

`openpods status --watch` streams live updates from the daemon. See
[`docs/linux-bluetooth.md`](docs/linux-bluetooth.md) for the i3/Wayland tray
caveats.

Target toolchain (dev host): Go ≥ 1.25, BlueZ ≥ 5.72, Linux with `org.bluez` on
the system D-Bus.

The remaining frontends (tray, GUI) are being built incrementally following the
design; [`PROMPT.md`](PROMPT.md) is the kickoff prompt that points an AI coding
agent at the docs and the next phase.

## License

GPLv3 — see [`LICENSE`](LICENSE). This is a derivative of OpenPods (© 2019–2022
Federico Dossena). AirPods and Beats are trademarks of Apple Inc.
