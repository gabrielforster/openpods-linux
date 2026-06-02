// Package pods decodes the Apple "proximity pairing" BLE advertisement that
// AirPods and compatible Beats broadcast with their battery state.
//
// It is a pure, platform-independent port of the upstream OpenPods Android
// decoder (PodsStatus.java, Pod.java and the models/* hierarchy,
// GPLv3 © Federico Dossena). The byte/nibble layout is documented in
// docs/beacon-protocol.md, but where that prose and the Android code disagree
// the code is authoritative (see Decode for the left/right nibble note).
//
// No Bluetooth, D-Bus or UI imports live here: the package takes the 27-byte
// manufacturer payload and returns a Status.
package pods

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidPayload is returned by Decode for payloads that do not match the
// Apple proximity-pairing beacon shape (length 27, prefix 0x07 0x19).
var ErrInvalidPayload = errors.New("pods: invalid beacon payload")

// Battery-nibble constants, ported verbatim from Pod.java.
const (
	// DisconnectedStatus marks a pod/case that is not present or out of range.
	DisconnectedStatus = 15
	// MaxConnectedStatus is the highest level that counts as connected; it is
	// rendered as "100%".
	MaxConnectedStatus = 10
	// LowBatteryStatus is the threshold (inclusive) for the low-battery hint.
	LowBatteryStatus = 1
)

// Beacon validation constants (PodsStatusScanCallback.java / beacon-protocol §1).
const (
	payloadLen = 27
	prefix0    = 0x07 // Apple message type "proximity pairing"
	prefix1    = 0x19 // length of the remaining data (25)
)

// Hex-character offsets into the 54-char uppercase hex string (beacon-protocol §3).
const (
	idxModelLo = 6  // idFull occupies chars [6,10): chars 6,7,8,9
	idxModelHi = 10 // exclusive end of idFull
	idxSingle  = 7  // idSingle = char 7 (low nibble of byte 3)
	idxFlip    = 10 // flip flag nibble
	idxInEar   = 11 // in-ear nibble (bit 1 = left, bit 3 = right)
	idxBattA   = 12 // battery A (left when not flipped)
	idxBattB   = 13 // battery B (right when not flipped); also the single figure
	idxCharge  = 14 // charge nibble (bit 0 = left, bit 1 = right, bit 2 = case)
	idxCase    = 15 // case battery
)

// Pod is the decoded battery/charge/in-ear state of one earbud or the case.
type Pod struct {
	// Level is the raw battery nibble: 0..10 = battery level, 15 = disconnected,
	// 11..14 = unknown (treated as not connected).
	Level    int
	Charging bool
	InEar    bool
}

// Connected reports whether the pod holds a valid battery level (Level <= 10).
func (p Pod) Connected() bool { return p.Level <= MaxConnectedStatus }

// Disconnected reports the explicit "not present" sentinel (Level == 15).
func (p Pod) Disconnected() bool { return p.Level == DisconnectedStatus }

// Low reports the low-battery hint (Level <= 1). A disconnected pod is not low.
func (p Pod) Low() bool { return p.Level <= LowBatteryStatus }

// Percent returns the displayed battery estimate and whether it is valid.
// Level 10 -> 100; level 0..9 -> level*10+5; anything > 10 -> (0, false).
func (p Pod) Percent() (int, bool) {
	switch {
	case p.Level == MaxConnectedStatus:
		return 100, true
	case p.Level < MaxConnectedStatus:
		return p.Level*10 + 5, true
	default:
		return 0, false
	}
}

// Status is a fully decoded beacon.
//
// For stereo models Left, Right and Case hold the three figures. For single
// models (Single == true) the one battery figure is in Left, and Right/Case are
// set to a disconnected Pod. Flipped records whether Apple swapped the
// left/right nibbles for this beacon.
//
// There is intentionally no capture timestamp here: Decode is a pure function.
// The scanner (ble package) stamps capture time when it reads the beacon, which
// is what the staleness rule in core consumes.
type Status struct {
	Model             Model
	Single            bool
	Left, Right, Case Pod
	Flipped           bool
}

// Decode validates and decodes a 27-byte Apple manufacturer payload (the bytes
// BlueZ exposes under ManufacturerData key 0x004C, i.e. everything after the
// company id). It returns ErrInvalidPayload if the length or 0x07 0x19 prefix
// is wrong.
func Decode(payload []byte) (Status, error) {
	if len(payload) != payloadLen {
		return Status{}, fmt.Errorf("%w: length %d, want %d", ErrInvalidPayload, len(payload), payloadLen)
	}
	if payload[0] != prefix0 || payload[1] != prefix1 {
		return Status{}, fmt.Errorf("%w: prefix 0x%02X 0x%02X, want 0x%02X 0x%02X",
			ErrInvalidPayload, payload[0], payload[1], prefix0, prefix1)
	}

	// Match the Android convention exactly: uppercase hex string indexed by
	// character. Every character is a valid hex digit because it came from
	// EncodeToString, so nibble() never fails.
	s := strings.ToUpper(hex.EncodeToString(payload))

	// The flip flag is set when bit 1 of char 10 is CLEAR (PodsStatus.isFlipped).
	flipped := nibble(s[idxFlip])&0x02 == 0

	// NOTE: per the Android code (PodsStatus.java:50-51) the left battery is
	// read from char 13 and the right from char 12 when NOT flipped (and the
	// reverse when flipped). beacon-protocol.md §3 describes it the other way
	// round; the code wins.
	leftLevel := nibble(s[pick(flipped, idxBattA, idxBattB)])
	rightLevel := nibble(s[pick(flipped, idxBattB, idxBattA)])
	caseLevel := nibble(s[idxCase])
	singleLevel := nibble(s[idxBattB]) // single figure is always char 13

	charge := nibble(s[idxCharge])
	chargeLeft := charge&bit(flipped, 0b0010, 0b0001) != 0
	chargeRight := charge&bit(flipped, 0b0001, 0b0010) != 0
	chargeCase := charge&0b0100 != 0
	chargeSingle := charge&0b0001 != 0 // single charge is always bit 0

	inEar := nibble(s[idxInEar])
	inEarLeft := inEar&bit(flipped, 0b1000, 0b0010) != 0
	inEarRight := inEar&bit(flipped, 0b0010, 0b1000) != 0

	idFull := s[idxModelLo:idxModelHi]
	idSingle := s[idxSingle]
	model, single := detectModel(idFull, idSingle)

	st := Status{Model: model, Single: single, Flipped: flipped}
	if single {
		st.Left = Pod{Level: singleLevel, Charging: chargeSingle}
		st.Right = Pod{Level: DisconnectedStatus}
		st.Case = Pod{Level: DisconnectedStatus}
	} else {
		st.Left = Pod{Level: leftLevel, Charging: chargeLeft, InEar: inEarLeft}
		st.Right = Pod{Level: rightLevel, Charging: chargeRight, InEar: inEarRight}
		st.Case = Pod{Level: caseLevel, Charging: chargeCase}
	}
	return st, nil
}

// detectModel maps the model id to a Model and reports whether it is a
// single-figure device. The match order mirrors PodsStatus.java exactly because
// idSingle (char 7) overlaps idFull (chars 6-9).
func detectModel(idFull string, idSingle byte) (Model, bool) {
	switch {
	case idFull == "0220":
		return ModelAirPods1, false
	case idFull == "0F20":
		return ModelAirPods2, false
	case idFull == "1320":
		return ModelAirPods3, false
	case idFull == "0E20":
		return ModelAirPodsPro, false
	case idFull == "1420" || idFull == "2420":
		return ModelAirPodsPro2, false
	case idFull == "2720":
		return ModelAirPodsPro3, false
	case idSingle == 'A':
		return ModelAirPodsMax, true
	case idSingle == 'B':
		return ModelPowerbeatsPro, false
	case idFull == "0520":
		return ModelBeatsX, true
	case idFull == "1020":
		return ModelBeatsFlex, true
	case idFull == "0620":
		return ModelBeatsSolo3, true
	case idSingle == '9':
		return ModelBeatsStudio3, true
	case idFull == "0320":
		return ModelPowerbeats3, true
	default:
		return ModelUnknown, false
	}
}

// nibble returns the value 0..15 of a single uppercase hex digit.
func nibble(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	default:
		return 0
	}
}

// pick returns a when flipped, else b.
func pick(flipped bool, a, b int) int {
	if flipped {
		return a
	}
	return b
}

// bit returns mask a when flipped, else mask b.
func bit(flipped bool, a, b int) int {
	if flipped {
		return a
	}
	return b
}
