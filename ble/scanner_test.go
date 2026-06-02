package ble_test

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"openpods-linux/ble"
	"openpods-linux/pods"
)

// validPayload returns a 27-byte beacon that decodes to AirPods Pro with
// Left=5 (55%), Right=10 (100%), Case=8 (85%) — the Phase 0 reference vector.
func validPayload(t *testing.T) []byte {
	t.Helper()
	p, err := hex.DecodeString("0719010E2020A508" + strings.Repeat("00", 19))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// fakeSource is an in-memory Source driven by the test.
type fakeSource struct {
	beacons chan ble.Beacon
	conns   chan ble.ConnEvent
}

func newFakeSource() *fakeSource {
	return &fakeSource{beacons: make(chan ble.Beacon), conns: make(chan ble.ConnEvent)}
}

func (f *fakeSource) Beacons() <-chan ble.Beacon        { return f.beacons }
func (f *fakeSource) Connections() <-chan ble.ConnEvent { return f.conns }
func (f *fakeSource) Close() error                      { return nil }

func TestScannerEmitsDecodedStatus(t *testing.T) {
	src := newFakeSource()
	sc := ble.NewScanner(src, ble.DefaultMinRSSI, ble.DefaultRecentWindow)
	defer sc.Close()

	src.beacons <- ble.Beacon{Address: "AA:BB:CC", Data: validPayload(t), RSSI: -50, Time: time.Now()}

	select {
	case st := <-sc.Updates():
		if st.Model != pods.ModelAirPodsPro {
			t.Errorf("Model = %s, want airpodspro", st.Model)
		}
		if p, ok := st.Left.Percent(); !ok || p != 55 {
			t.Errorf("Left.Percent() = (%d,%v), want (55,true)", p, ok)
		}
		if p, ok := st.Right.Percent(); !ok || p != 100 {
			t.Errorf("Right.Percent() = (%d,%v), want (100,true)", p, ok)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for a status update")
	}
}

func TestScannerDropsWeakBeacon(t *testing.T) {
	src := newFakeSource()
	sc := ble.NewScanner(src, ble.DefaultMinRSSI, ble.DefaultRecentWindow)
	defer sc.Close()

	// -70 dBm is below the -60 floor; nothing should be emitted.
	src.beacons <- ble.Beacon{Address: "AA:BB:CC", Data: validPayload(t), RSSI: -70, Time: time.Now()}

	select {
	case st := <-sc.Updates():
		t.Fatalf("weak beacon should be dropped, got %s", st.Model)
	case <-time.After(150 * time.Millisecond):
		// expected: no emission
	}
}

func TestScannerTracksConnection(t *testing.T) {
	src := newFakeSource()
	sc := ble.NewScanner(src, ble.DefaultMinRSSI, ble.DefaultRecentWindow)
	defer sc.Close()

	src.conns <- ble.ConnEvent{Address: "AA:BB:CC", Connected: true}
	if got := recvBool(t, sc.Connected()); !got {
		t.Errorf("Connected = %v, want true", got)
	}

	src.conns <- ble.ConnEvent{Address: "AA:BB:CC", Connected: false}
	if got := recvBool(t, sc.Connected()); got {
		t.Errorf("Connected = %v, want false", got)
	}
}

func TestScanReturnsFirstStatus(t *testing.T) {
	src := newFakeSource()
	go func() {
		src.beacons <- ble.Beacon{Address: "AA:BB:CC", Data: validPayload(t), RSSI: -50, Time: time.Now()}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	st, ok := ble.Scan(ctx, src, ble.DefaultMinRSSI, ble.DefaultRecentWindow)
	if !ok {
		t.Fatal("Scan returned ok=false, want a status")
	}
	if st.Model != pods.ModelAirPodsPro {
		t.Errorf("Model = %s, want airpodspro", st.Model)
	}
}

func TestScanReturnsFalseWhenSourceStops(t *testing.T) {
	beacons := make(chan ble.Beacon)
	close(beacons) // source produces nothing and is already done
	src := &fakeSource{beacons: beacons, conns: make(chan ble.ConnEvent)}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, ok := ble.Scan(ctx, src, ble.DefaultMinRSSI, ble.DefaultRecentWindow); ok {
		t.Error("Scan returned ok=true, want false when the source stops")
	}
}

func TestScanTimesOutWithoutBeacons(t *testing.T) {
	src := newFakeSource()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, ok := ble.Scan(ctx, src, ble.DefaultMinRSSI, ble.DefaultRecentWindow); ok {
		t.Error("Scan returned ok=true, want false on timeout")
	}
}

func TestScannerStopsWhenBeaconChannelCloses(t *testing.T) {
	beacons := make(chan ble.Beacon)
	close(beacons)
	src := &fakeSource{beacons: beacons, conns: make(chan ble.ConnEvent)}
	sc := ble.NewScanner(src, ble.DefaultMinRSSI, ble.DefaultRecentWindow)
	defer sc.Close()
	assertScannerStopped(t, sc)
}

func TestScannerStopsWhenConnChannelCloses(t *testing.T) {
	conns := make(chan ble.ConnEvent)
	close(conns)
	src := &fakeSource{beacons: make(chan ble.Beacon), conns: conns}
	sc := ble.NewScanner(src, ble.DefaultMinRSSI, ble.DefaultRecentWindow)
	defer sc.Close()
	assertScannerStopped(t, sc)
}

// assertScannerStopped waits for the scanner to close its Updates channel,
// which it does when the run loop exits (here, because the source closed).
func assertScannerStopped(t *testing.T, sc ble.Scanner) {
	t.Helper()
	select {
	case _, ok := <-sc.Updates():
		if ok {
			t.Fatal("expected Updates to be closed on stop, got a value")
		}
	case <-time.After(time.Second):
		t.Fatal("scanner did not stop after the source closed")
	}
}

func recvBool(t *testing.T, ch <-chan bool) bool {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for a connection update")
		return false
	}
}
