# Linux Bluetooth: BlueZ, Beacons, and Desktop Caveats

How the Android BLE concepts translate to Linux, the gotchas that differ from
Android, and the per-desktop realities for the tray and notifications.

## BlueZ over D-Bus, in one picture

On Linux the Bluetooth stack is **BlueZ**, controlled over **D-Bus** on the
**system bus** under the well-known name `org.bluez`. There is no direct
"BLE scanner callback" like Android's; instead BlueZ models nearby devices as
D-Bus objects and you read their properties / subscribe to changes.

Key objects and members we use:

| D-Bus interface | Member | Use |
| --- | --- | --- |
| `org.bluez.Adapter1` | `SetDiscoveryFilter(a{sv})` | restrict to LE, keep duplicates |
| `org.bluez.Adapter1` | `StartDiscovery()` / `StopDiscovery()` | begin/end scanning |
| `org.bluez.Adapter1` | prop `Powered` | detect BT on/off |
| `org.bluez.Device1` | prop `ManufacturerData a{qv}` | the beacon payload (key `0x004C`) |
| `org.bluez.Device1` | prop `RSSI n` | signal strength for the heuristic |
| `org.bluez.Device1` | prop `Connected b`, `UUIDs as` | "are my AirPods actually connected" gate |
| `org.freedesktop.DBus.ObjectManager` | `InterfacesAdded`, `GetManagedObjects` | discover devices/adapters |
| `org.freedesktop.DBus.Properties` | `PropertiesChanged` | live updates of the props above |

In Go we talk to this directly with `github.com/godbus/dbus/v5`. It keeps
dependencies minimal and gives full control over reading `ManufacturerData`/
`RSSI`. (`github.com/muka/go-bluetooth` and `tinygo.org/x/bluetooth` are
higher-level wrappers around the same calls; viable, but the raw client is
preferred for the scanner — see [`roadmap.md`](./roadmap.md) Phase 1.)

### Reading the beacon

The Android filter checks Apple manufacturer id `76` with payload bytes
`[0]=0x07, [1]=0x19`, length 27. On BlueZ the equivalent is: read the
`ManufacturerData` map of each discovered `Device1`, take the value at key
`0x004C` (a `[]byte`), and apply the **same** validation and decode from
[`beacon-protocol.md`](./beacon-protocol.md). The bytes BlueZ gives you are the
payload *after* the company id — identical to what Android's
`getManufacturerSpecificData(76)` returns.

### Discovery filter

```
SetDiscoveryFilter({
  "Transport":     "le",     // LE only
  "DuplicateData": true,     // keep getting advertisement updates (battery changes)
  "RSSI":          int16(-70) // coarse pre-filter; fine -60 gate applied in code
})
StartDiscovery()
```

`DuplicateData:true` is important: without it BlueZ may coalesce repeats and we'd
miss battery changes. We still apply the precise `RSSI ≥ -60` gate ourselves to
match the Android behavior.

> Note: while discovery is on, BlueZ accumulates `Device1` objects for every
> nearby BLE device. The daemon should periodically `RemoveDevice` stale
> non-matching entries (or rely on BlueZ's own cleanup) to avoid unbounded
> growth on busy RF environments.

### Alternative: `AdvertisementMonitor` (BlueZ ≥ 5.x, present on 5.72)

`org.bluez.AdvertisementMonitorManager1.RegisterMonitor` lets you register a
pattern (e.g. manufacturer-data prefix `4C 00 07 19`) and have BlueZ call
`DeviceFound`/`DeviceLost` on a callback object — more power-efficient and no
active-discovery churn. It is the "right" long-term API for a passive monitor,
but it's slightly more complex and historically gated behind the `Experimental`
flag on older BlueZ. **Plan:** ship Phase 1 with plain discovery (works
everywhere), then offer `AdvertisementMonitor` as an opt-in/auto-detected
optimization later. Documented as a roadmap item, not v1 scope.

## The "which AirPods are mine?" problem

Apple sends the status beacon from a **rotating random BLE address** that is
*not* the address of the paired audio device. So — exactly as on Android — we
cannot simply filter beacons by our AirPods' MAC.

**Ported heuristic (from `PodsStatusScanCallback`):** keep beacons seen in the
last 10 s, pick the one with the strongest RSSI, and ignore anything weaker than
−60 dBm. The assumption: the loudest AirPods beacon nearby is yours.

**Linux-only improvement:** unlike Android, BlueZ *does* let us see device
addresses and connection state. We additionally track whether an AirPods audio
device is actually `Connected` (matching the AirPods service UUIDs the Android
app already knows —
`74ec2172-0bad-4d01-8f77-997b2be0722a` and its byte-reversed form
`2a72e02b-7b99-778f-014d-ad0b7221ec74`). We only treat status as "yours" while
such a device is connected. This is the Linux analogue of Android's
`mMaybeConnected` flag and meaningfully reduces false positives from a
neighbour's AirPods on the same bus/train.

This does **not** fully solve attribution (the beacon address still differs from
the audio device address), but combining *connected-gate* + *strongest-recent +
RSSI≥-60* is at least as good as Android and better in the common case.

## Permissions

On most desktop distros a normal user in an active `systemd-logind` session can
drive BlueZ discovery via polkit without extra setup. If discovery is denied:

- Ensure the user is in the `bluetooth` group (some distros), or
- Add a polkit rule allowing `org.bluez` actions for active local sessions.

The daemon surfaces a single actionable error (not a silent failure) when a
discovery call is denied. Document the polkit snippet in the project README.

## Notifications

`org.freedesktop.Notifications.Notify` on the **session** bus is the freedesktop
standard and works under any notification daemon (GNOME Shell, Plasma,
**dunst** on i3, mako on wlroots, xfce4-notifyd, …). We call it directly via
godbus — no extra dependency. We fire on connect/disconnect and (optionally) a
single low-battery edge, with the app icon and a short body
("AirPods Pro — L 85% · R 90% · Case 55%"). This is robust and the same on every
target desktop.

## System-tray reality check (read this before building the tray)

The tray is the one frontend whose behavior genuinely differs across desktops,
because two incompatible protocols exist:

| Protocol | Used by | Go support |
| --- | --- | --- |
| **StatusNotifierItem (SNI)** over D-Bus | KDE Plasma, waybar, GNOME *with* the AppIndicator extension | `fyne.io/systray` produces an SNI item |
| **XEmbed** (old freedesktop System Tray) | **i3bar**, **polybar**'s `tray` module, stalonetray, xfce4 (legacy) | not what `fyne/systray` speaks |

Consequences for our targets:

- **KDE Plasma**: SNI works natively. Best case — the tray "just works".
- **GNOME**: no native tray at all; needs the *AppIndicator and KStatusNotifier
  Support* extension. With it, SNI works; without it, the tray icon won't appear.
- **waybar (wlroots/Wayland)**: has an SNI `tray` module — works.
- **i3 on X11 (the dev host)** and **polybar**: their trays are **XEmbed**, so a
  `fyne/systray` **SNI item will NOT appear** in i3bar/polybar unless you run a
  bridge such as **`snixembed`** (an SNI→XEmbed proxy). This is the key
  takeaway: *on your machine the SNI tray is not the natural display.*

### Recommendation per environment

- **i3 / polybar / tiling setups (your primary)**: use the **CLI/`--waybar`
  frontend** as the always-visible display — feed `i3status-rust`, `i3blocks`,
  or polybar a `custom/script` that runs `openpods status --waybar` (or
  `--json`) every few seconds. Plus `libnotify` notifications. This needs no
  tray protocol at all and is the most reliable. Optionally document `snixembed`
  for users who want the SNI icon in i3bar.
- **KDE / GNOME(+extension) / waybar**: the `fyne/systray` tray icon is a
  first-class display.
- The **GUI window** is environment-agnostic (it's just an app window) and works
  everywhere as the "detailed view", launched from the bar/tray/CLI.

Because the project must be portable (per the scope decision), we ship all
frontends and let each environment use whichever fits — but we **document i3 as
CLI-first** so the dev host has a great experience out of the box.

## Example: i3 status-bar integration (i3blocks)

```ini
# ~/.config/i3blocks/config
[openpods]
command=openpods status --waybar | jq -r '.text'
interval=5
```

Or polybar:

```ini
[module/openpods]
type = custom/script
exec = openpods status --waybar | jq -r .text
interval = 5
```

The daemon makes this cheap: the command just reads the last-known status from
the Unix socket; it does not start a scan per refresh.
