# Android Reference — Knowledge Base

Verbatim copies of the relevant source from the upstream **OpenPods** Android app
(GPLv3, © Federico Dossena), kept here as **ground truth** for the Linux port.
When the design docs and this code disagree, **the code wins** — update the docs.

Everything here is *reference only*: it is Java/Android and is **not compiled**
by this project. The Go port re-implements the portable logic and replaces the
Android-specific parts (see the mapping table below).

## Why this exists

The Linux port is a derivative work. Rather than make the implementer cross-
reference a separate Gradle/Android repo, the files that actually matter for a
faithful port are copied here. The single most important artifact —
the decoded beacon format — is also written up independently in
[`../beacon-protocol.md`](../beacon-protocol.md).

## File index

### `pods/` — core logic + scanning

| File | Responsibility | Portability |
| --- | --- | --- |
| `PodsStatus.java` | **The decoder.** Turns the 27-byte Apple manufacturer payload (as a hex string) into battery/charge/in-ear/model. Contains the canonical field-offset comments. | **100% portable** → Go `pods` package. The class Javadoc is the original protocol write-up. |
| `Pod.java` | Value object for one earbud/case: battery nibble (0–10, 15=disconnected), charging, in-ear; display rule `level*10+5`%; constants `DISCONNECTED=15`, `MAX_CONNECTED=10`, `LOW_BATTERY=1`. | **100% portable** (drop the `View`/`R.drawable` UI helpers). |
| `PodsStatusScanCallback.java` | BLE scan callback + filter. Defines manufacturer id `76`, payload length `27`, prefix bytes `0x07 0x19`, `MIN_RSSI=-60`, and the **strongest-recent-beacon (10s) MAC workaround**. | Logic portable; the `ScanFilter`/`ScanResult` plumbing → BlueZ in Go `ble` package. |
| `PodsService.java` | The Android `Service`: starts/stops the scanner on BT and screen events, tracks "maybe connected" via ACL broadcasts + `BluetoothProfile.HEADSET` proxy, holds the AirPods service **UUIDs**. | Concept maps to the Go `openpodsd` daemon + `core` state. Android service/receiver machinery is replaced by BlueZ `PropertiesChanged`. |
| `models/` | The `IPods` hierarchy: `RegularPods` (L/R/case), `SinglePods` (one figure), and one tiny subclass per model selecting artwork + model name. `Constants.java` lists the model id strings. | Logic + model table portable → Go `pods` model enum. Artwork selection maps to the embedded assets. |

### `notification/` — display loop

| File | Responsibility | Portability |
| --- | --- | --- |
| `NotificationThread.java` | Polls status every **1 s**; shows/updates/clears the notification while connected. | Concept maps to the daemon's update loop + frontends. |
| `NotificationBuilder.java` | Renders the custom notification; defines **`TIMEOUT_CONNECTED=30000`** (battery hidden / "updating" if status older than 30 s). | The 30 s staleness rule is portable and lives in Go `core`; the `RemoteViews` rendering is replaced by tray/GUI/notifications. |

### `drawable/` — pod artwork

PNGs for each model and a `_disconnected` variant (`pod`, `podpro`, `podpro3`,
`podmax`, `pod_case`, `podpro_case`, `powerbeatspro(_case)`, `beatsx`,
`beatsflex`, `beatssolo3`, `beatsstudio3`, `powerbeats3`). Plus `icon.png` /
`logo.png` app branding (incidental). Reused by the Go tray/GUI (Phases 3–4),
embedded via `//go:embed`. The model→image mapping is in the `models/` classes.

> Battery state icons (`ic_battery_*`) are Android vector XML and are **not**
> copied; recreate equivalents in the GUI as needed.

## The essential facts (extracted)

These are the load-bearing constants the port must preserve. Full detail with
bit math is in [`../beacon-protocol.md`](../beacon-protocol.md).

- Apple company id: **`0x004C` (76)**; payload length **27**; prefix **`0x07 0x19`**.
- Battery nibble: `0–10` → `level*10+5`% (`10`→"100%"); `15` → disconnected.
- Flip flag at hex char 10 swaps L/R for battery, charge, and in-ear bits.
- Field hex-char offsets: model `6–9` (`idSingle` = char 7), flip `10`, in-ear
  `11`, battery `12`/`13`, charge `14`, case `15`.
- Attribution heuristic: strongest beacon in the last **10 s**, **RSSI ≥ −60 dBm**.
- Staleness: no beacon for **30 s** → hide battery figures.
- AirPods service UUIDs (used by Android to confirm a connected device):
  `74ec2172-0bad-4d01-8f77-997b2be0722a` and its reversed form
  `2a72e02b-7b99-778f-014d-ad0b7221ec74`.

## Android → Go mapping (quick reference)

| Android | Go |
| --- | --- |
| `PodsStatus` + `Pod` + `models/*` | `pods` package (pure, tested) |
| `PodsStatusScanCallback` | `ble` package (BlueZ/D-Bus) |
| `PodsService` (+ receivers, proxy) | `openpodsd` daemon + `core` state |
| `NotificationThread`/`NotificationBuilder` | `core` update loop + `notify`/tray/GUI frontends |
| `res/drawable/*.png` | embedded `assets` |

## License

This reference code and the artwork are GPLv3 (see [`../../LICENSE`](../../LICENSE)).
The Linux port, as a derivative work, is also GPLv3. Preserve the original
copyright attribution to **Federico Dossena** and link back to the upstream
project.
