package ble

import "time"

// Beacon is one raw Apple manufacturer-data advertisement observed from a BLE
// device at a moment in time. Data is the value BlueZ exposes under
// ManufacturerData key 0x004C — identical to what Android's
// getManufacturerSpecificData(76) returns (the bytes after the company id).
type Beacon struct {
	Address string    // BLE device address (a rotating random address for AirPods)
	Data    []byte    // ManufacturerData[0x004C]
	RSSI    int16     // signal strength in dBm
	Time    time.Time // when the advertisement was observed
}

// Heuristic parameters ported from PodsStatusScanCallback.java.
const (
	// DefaultMinRSSI is the floor for accepting a beacon (MIN_RSSI = -60).
	DefaultMinRSSI int16 = -60
	// DefaultRecentWindow is how long a beacon stays eligible as the "strongest
	// recent" candidate (RECENT_BEACONS_MAX_T_NS = 10s).
	DefaultRecentWindow = 10 * time.Second
)

// selector implements the "which beacon is mine" workaround from
// PodsStatusScanCallback: Apple advertises battery from a rotating random
// address, so we keep beacons seen within a recent window, take the strongest
// by RSSI, prefer the freshest reading when it shares the newest beacon's
// address, and reject anything weaker than the RSSI floor.
type selector struct {
	minRSSI int16
	window  time.Duration
	recent  []Beacon
}

func newSelector(minRSSI int16, window time.Duration) *selector {
	return &selector{minRSSI: minRSSI, window: window}
}

// best records b, prunes beacons older than the window (relative to b's
// arrival time), and returns the chosen beacon plus whether it clears the RSSI
// gate. It mirrors getBestResult followed by the `rssi < MIN_RSSI` check.
func (s *selector) best(b Beacon) (Beacon, bool) {
	s.recent = append(s.recent, b)

	// Prune beacons older than the window. The just-added beacon is the
	// reference "now"; it always survives, so recent is never empty below.
	now := b.Time
	kept := s.recent[:0]
	for _, rb := range s.recent {
		if now.Sub(rb.Time) <= s.window {
			kept = append(kept, rb)
		}
	}
	s.recent = kept

	// Strongest by RSSI (first max wins on ties, matching the Java `<`).
	strongest := s.recent[0]
	for _, rb := range s.recent[1:] {
		if rb.RSSI > strongest.RSSI {
			strongest = rb
		}
	}

	// If the strongest beacon is from the same device as the newest arrival,
	// use the fresh reading (latest battery state) for that device.
	chosen := strongest
	if chosen.Address == b.Address {
		chosen = b
	}

	if chosen.RSSI < s.minRSSI {
		return Beacon{}, false
	}
	return chosen, true
}
