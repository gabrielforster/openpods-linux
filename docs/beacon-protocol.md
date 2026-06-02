# AirPods BLE Beacon Protocol

This is the reverse-engineered format of the Apple "proximity" BLE advertisement
that carries AirPods/Beats battery status. It is transcribed directly from the
Android implementation (`PodsStatus.java`, `Pod.java`,
`PodsStatusScanCallback.java`) so the Go port can be a faithful, verifiable
re-implementation. **This is the single source of truth for decoding** — both
the Android and Linux projects should track it here.

> ⚠️ Unofficial. Apple publishes none of this. It was derived by observation and
> may be wrong or incomplete for some models/firmware. Treat unknown values
> defensively.

## 1. Identifying the packet

AirPods advertise BLE manufacturer-specific data under **Apple's company ID
`0x004C` (76 decimal)**.

The payload that follows the company ID is **exactly 27 bytes** and begins with
a fixed prefix:

| Byte | Value | Meaning |
| --- | --- | --- |
| `0` | `0x07` | Apple message type = "proximity pairing" |
| `1` | `0x19` (25) | Length of the remaining data |
| `2..26` | … | Status payload (see below) |

Validation (all three must hold), matching the Android scan filter:

```
len(payload) == 27  &&  payload[0] == 0x07  &&  payload[1] == 0x19
```

> On Android the scan filter is expressed as `manufacturerData[0]=7`,
> `manufacturerData[1]=25` with a full mask on those two bytes. On Linux, BlueZ
> exposes the same payload as the value of key `0x004C` in the device's
> `ManufacturerData` map — see [`linux-bluetooth.md`](./linux-bluetooth.md).

## 2. From bytes to a hex string

The Android code converts the 27-byte payload to a **54-character uppercase hex
string** (`%02X` per byte) and then indexes individual **characters**. We keep
the same convention so the position numbers below match the original source
exactly.

```
hexIndex i  ->  byte = payload[i / 2],  nibble = high if i even else low
```

So character `12` is the high nibble of byte 6, character `13` is the low nibble
of byte 6, and so on.

## 3. Field positions (0-indexed into the 54-char hex string)

| Hex char | Field | Notes |
| --- | --- | --- |
| `6,7,8,9` | **Model ID** (`idFull`, 4 hex chars) | e.g. `0E20` = AirPods Pro |
| `7` | **Single-device model char** (`idSingle`) | low nibble of byte 3; used for Max / Powerbeats Pro / Studio 3 |
| `10` | **Flip flag** | `flipped = (val & 0x02) == 0` |
| `11` | **In-ear status** | bit 1 = left, bit 3 = right (swapped if flipped) |
| `12` | **Battery A** | right if not flipped, left if flipped |
| `13` | **Battery B** | left if not flipped, right if flipped; also the "single" battery |
| `14` | **Charging bits** | bit 0 = left, bit 1 = right, bit 2 = case (left/right swap if flipped) |
| `15` | **Case battery** | |

> ⚠️ **Left/right battery, reconciled with the code.** The authoritative source
> `PodsStatus.java:48-51` reads `left = charAt(flip ? 12 : 13)` and
> `right = charAt(flip ? 13 : 12)`, so **when *not* flipped the left figure comes
> from char 13 and the right from char 12** (and the reverse when flipped). The
> upstream class Javadoc prose ("the 12th and 13th characters … left and right")
> says the opposite; the table above follows the *code*, which is ground truth.
> Charging (char 14) and in-ear (char 11) keep their natural mapping (bit 0/bit 1
> = left/right, bit 1/bit 3 = left/right) in the not-flipped case.

### The "flip" flag

Under some conditions (believed to relate to in-ear detection) Apple swaps the
left/right nibbles. The flip flag is read from hex char `10`:

```
flipped = (hexval(char[10]) & 0x02) == 0
```

When flipped, swap left↔right for the battery nibbles (chars 12/13), the
charging bits (0↔1), and the in-ear bits (1↔3).

## 4. Interpreting a battery nibble

Each battery nibble is a single hex digit `0x0`–`0xF`:

| Nibble value | Meaning |
| --- | --- |
| `0`–`10` | Battery level. Displayed estimate = `value * 10 + 5` % (and `10` → "100%") |
| `15` (`0xF`) | Disconnected / not present |
| other (`11`–`14`) | Treated as not connected (unknown) |

Reference constants from `Pod.java`:

```
DISCONNECTED_STATUS = 15   // nibble == 15  -> disconnected
MAX_CONNECTED_STATUS = 10  // nibble <= 10  -> connected;  == 10 shows "100%"
LOW_BATTERY_STATUS = 1     // nibble <= 1   -> low-battery indicator
```

Display rule (`Pod.parseStatus()`):

```
nibble == 10  -> "100%"
nibble  < 10  -> (nibble*10 + 5) "%"     // 0 -> "5%", 5 -> "55%", 9 -> "95%"
nibble  > 10  -> ""                       // unknown / disconnected
```

## 5. Charging & in-ear bits

```
chargeNibble = hexval(char[14])
chargeLeft  = chargeNibble & (flipped ? 0b0010 : 0b0001)
chargeRight = chargeNibble & (flipped ? 0b0001 : 0b0010)
chargeCase  = chargeNibble & 0b0100

inEarNibble = hexval(char[11])
inEarLeft  = inEarNibble & (flipped ? 0b1000 : 0b0010)
inEarRight = inEarNibble & (flipped ? 0b0010 : 0b1000)
```

(For single-device models the "single" battery is char `13` and its charging bit
is `chargeNibble & 0b0001`.)

## 6. Model detection table

Matched in this order (`idFull` is chars 6–9; `idSingle` is char 7):

| Match | Model | Type |
| --- | --- | --- |
| `idFull == "0220"` | AirPods (1st gen) | stereo (L/R/case) |
| `idFull == "0F20"` | AirPods (2nd gen) | stereo |
| `idFull == "1320"` | AirPods (3rd gen) | stereo |
| `idFull == "0E20"` | AirPods Pro | stereo |
| `idFull == "1420"` or `"2420"` | AirPods Pro 2 | stereo |
| `idFull == "2720"` | AirPods Pro 3 | stereo |
| `idSingle == 'A'` | AirPods Max | single |
| `idSingle == 'B'` | Powerbeats Pro | stereo |
| `idFull == "0520"` | Beats X | single |
| `idFull == "1020"` | Beats Flex | single |
| `idFull == "0620"` | Beats Solo 3 | single |
| `idSingle == '9'` | Beats Studio 3 | single |
| `idFull == "0320"` | Powerbeats 3 | single |
| *(anything else)* | Unknown | treated as stereo (L/R/case) |

> "single" models report one battery figure (rendered in the case slot in the
> Android UI); "stereo" models report left, right, and case.

## 7. Worked example

Suppose BlueZ hands us `ManufacturerData[0x004C]` =
`07 19 01 0E 20 2B 03 08 8F 04 31 00 ...` (27 bytes total). Hex string =
`"07190120E2B03088F0431..."` — *(illustrative; positions are what matter)*.

1. Validate: length 27, `[0]==0x07`, `[1]==0x19`. ✓
2. `idFull` = chars 6–9. `idSingle` = char 7.
3. `flipped` = `(hexval(char[10]) & 0x02) == 0`.
4. `left` = char 13 (or 12 if flipped); `right` = char 12 (or 13 if flipped); `case` = char 15.
5. Convert each nibble per §4; apply charging/in-ear per §5; pick model per §6.

The Go `pods` package implements exactly these steps as pure functions, with the
positions above frozen as named constants and covered by table-driven tests
(see [`roadmap.md`](./roadmap.md#testing-strategy)).

## 8. Reference test vectors

Capture real payloads on the dev host with:

```sh
# while AirPods are out of the case and advertising
sudo btmon            # watch for the 4C 00 07 19 ... manufacturer data
# or, via BlueZ properties once discovered:
busctl --user introspect org.bluez /org/bluez/hci0/dev_XX_XX_XX_XX_XX_XX
```

Each captured `(hexstring -> expected decode)` pair becomes a unit-test row.
Include at least: one stereo model, one single model, a flipped sample, a
charging sample, and one with a `15` (disconnected) nibble.
