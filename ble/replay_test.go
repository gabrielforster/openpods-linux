package ble_test

import (
	"context"
	"testing"
	"time"

	"openpods-linux/ble"
	"openpods-linux/pods"
)

func TestReplaySourceEmitsConnectionAndCyclesBeacons(t *testing.T) {
	a := ble.Beacon{Address: "a", Data: []byte("AAAA"), RSSI: -40}
	b := ble.Beacon{Address: "b", Data: []byte("BBBB"), RSSI: -45}
	src := ble.NewReplaySource([]ble.Beacon{a, b}, 0)
	defer src.Close()

	select {
	case c := <-src.Connections():
		if !c.Connected {
			t.Error("replay should announce a connected device")
		}
	case <-time.After(time.Second):
		t.Fatal("no connection event from replay source")
	}

	// Beacons cycle through the slice and are stamped with a fresh Time.
	want := []string{"a", "b", "a", "b"}
	for i, w := range want {
		select {
		case got := <-src.Beacons():
			if got.Address != w {
				t.Errorf("beacon %d: Address = %q, want %q", i, got.Address, w)
			}
			if got.Time.IsZero() {
				t.Errorf("beacon %d: Time was not stamped", i)
			}
		case <-time.After(time.Second):
			t.Fatalf("beacon %d: timed out", i)
		}
	}
}

// TestReplaySourceDrivesScanner exercises the whole offline pipeline:
// replay source -> Scanner -> selector -> pods.Decode.
func TestReplaySourceDrivesScanner(t *testing.T) {
	src := ble.NewReplaySource([]ble.Beacon{{Address: "replay", Data: validPayload(t), RSSI: -40}}, 0)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	st, ok := ble.Scan(ctx, src, ble.DefaultMinRSSI, ble.DefaultRecentWindow)
	if !ok {
		t.Fatal("expected a decoded status from the replay source")
	}
	if st.Model != pods.ModelAirPodsPro {
		t.Errorf("Model = %s, want airpodspro", st.Model)
	}
}

func TestReplaySourceWithNoBeaconsStillAnnouncesConnection(t *testing.T) {
	src := ble.NewReplaySource(nil, 0)
	defer src.Close()
	select {
	case c := <-src.Connections():
		if !c.Connected {
			t.Error("want connected=true")
		}
	case <-time.After(time.Second):
		t.Fatal("no connection event from empty replay source")
	}
}

func TestReplaySourcePacesWithInterval(t *testing.T) {
	src := ble.NewReplaySource([]ble.Beacon{{Address: "a", Data: []byte("X")}}, 5*time.Millisecond)
	defer src.Close()
	<-src.Connections()
	start := time.Now()
	for i := range 2 {
		select {
		case <-src.Beacons():
		case <-time.After(time.Second):
			t.Fatalf("beacon %d: timed out", i)
		}
	}
	if elapsed := time.Since(start); elapsed < 5*time.Millisecond {
		t.Errorf("two beacons arrived in %v, want >= one interval", elapsed)
	}
}

func TestDemoBeaconsDecodeWithChargingAndInEar(t *testing.T) {
	bs := ble.DemoBeacons()
	if len(bs) == 0 {
		t.Fatal("DemoBeacons returned none")
	}
	if bs[0].RSSI < ble.DefaultMinRSSI {
		t.Errorf("RSSI %d is below the gate %d; replay would be filtered out", bs[0].RSSI, ble.DefaultMinRSSI)
	}

	st, err := pods.Decode(bs[0].Data)
	if err != nil {
		t.Fatalf("demo beacon failed to decode: %v", err)
	}
	if st.Model != pods.ModelAirPodsPro {
		t.Errorf("Model = %s, want airpodspro", st.Model)
	}
	if p, _ := st.Left.Percent(); p != 55 || !st.Left.Charging || !st.Left.InEar {
		t.Errorf("Left = %+v, want 55%% charging+in-ear", st.Left)
	}
	if p, _ := st.Right.Percent(); p != 100 || st.Right.Charging || !st.Right.InEar {
		t.Errorf("Right = %+v, want 100%% in-ear (not charging)", st.Right)
	}
	if p, _ := st.Case.Percent(); p != 85 || !st.Case.Charging {
		t.Errorf("Case = %+v, want 85%% charging", st.Case)
	}
}

func TestReplaySourceCloseDoesNotHang(t *testing.T) {
	src := ble.NewReplaySource([]ble.Beacon{{Address: "a", Data: []byte("X")}}, time.Hour)
	done := make(chan error, 1)
	go func() { done <- src.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close hung")
	}
}
