package ble

import (
	"testing"
	"time"
)

// TestSelectorBest exercises the strongest-recent-beacon heuristic and the
// RSSI gate ported from PodsStatusScanCallback.getBestResult / onScanResult.
// The reference "now" for staleness is the timestamp of the beacon being added
// (a beacon is processed the instant it arrives), so no separate clock is
// needed.
func TestSelectorBest(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	b := func(addr string, rssi int16, at time.Time) Beacon {
		return Beacon{Address: addr, RSSI: rssi, Time: at, Data: []byte(addr)}
	}

	type step struct {
		in       Beacon
		wantOK   bool
		wantAddr string // chosen device address (when ok)
		wantRSSI int16  // chosen RSSI (when ok) — disambiguates fresh vs old reading
	}
	tests := []struct {
		name  string
		steps []step
	}{
		{"single above threshold", []step{
			{b("X", -50, t0), true, "X", -50},
		}},
		{"single below threshold is rejected", []step{
			{b("X", -70, t0), false, "", 0},
		}},
		{"exactly at threshold is accepted", []step{
			{b("X", -60, t0), true, "X", -60},
		}},
		{"stronger recent beacon from another device wins", []step{
			{b("X", -50, t0), true, "X", -50},
			{b("Y", -70, t0.Add(time.Second)), true, "X", -50}, // Y arrives but X is stronger & recent
		}},
		{"fresh reading from same device overrides stronger old reading", []step{
			{b("X", -50, t0), true, "X", -50},
			{b("X", -55, t0.Add(time.Second)), true, "X", -55}, // same addr -> use the fresh (weaker) one
		}},
		{"stale beacon is pruned, then the gate fails", []step{
			{b("X", -50, t0), true, "X", -50},
			{b("Y", -65, t0.Add(11*time.Second)), false, "", 0}, // X older than 10s -> pruned; Y too weak
		}},
		{"strong recent beacon persists within the window", []step{
			{b("X", -50, t0), true, "X", -50},
			{b("Y", -65, t0.Add(5*time.Second)), true, "X", -50}, // X still within 10s and strongest
		}},
		{"a newer, stronger beacon becomes the choice", []step{
			{b("X", -60, t0), true, "X", -60},
			{b("Y", -50, t0.Add(time.Second)), true, "Y", -50}, // Y is stronger -> wins
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSelector(DefaultMinRSSI, DefaultRecentWindow)
			for i, st := range tt.steps {
				got, ok := s.best(st.in)
				if ok != st.wantOK {
					t.Fatalf("step %d: ok = %v, want %v", i, ok, st.wantOK)
				}
				if !ok {
					continue
				}
				if got.Address != st.wantAddr {
					t.Errorf("step %d: chosen Address = %q, want %q", i, got.Address, st.wantAddr)
				}
				if got.RSSI != st.wantRSSI {
					t.Errorf("step %d: chosen RSSI = %d, want %d", i, got.RSSI, st.wantRSSI)
				}
			}
		})
	}
}
