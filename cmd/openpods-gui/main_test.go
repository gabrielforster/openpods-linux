package main

import (
	"bytes"
	"testing"
	"time"

	"openpods-linux/ipc"
	"openpods-linux/pods"
)

func isPNG(b []byte) bool {
	return bytes.HasPrefix(b, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})
}

func snap(s pods.Status, connected, stale bool) ipc.Snapshot {
	return ipc.FromStatus(s, connected, stale, time.Unix(0, 0))
}

func TestViewModelStereo(t *testing.T) {
	vd := viewModel(snap(pods.Status{
		Model: pods.ModelAirPodsPro,
		Left:  pods.Pod{Level: 5},
		Right: pods.Pod{Level: 10, Charging: true},
		Case:  pods.Pod{Level: 8},
	}, true, false))

	if vd.Title != "AirPods Pro" {
		t.Errorf("Title = %q", vd.Title)
	}
	if len(vd.Cards) != 3 {
		t.Fatalf("len(Cards) = %d, want 3", len(vd.Cards))
	}
	if vd.Cards[0].Label != "Left" || vd.Cards[0].Value != "55%" || !isPNG(vd.Cards[0].Image) {
		t.Errorf("Left card = %+v", vd.Cards[0])
	}
	if vd.Cards[1].Value != "100%" || !vd.Cards[1].Charging {
		t.Errorf("Right card = %+v", vd.Cards[1])
	}
	if vd.Cards[2].Label != "Case" || vd.Cards[2].Value != "85%" {
		t.Errorf("Case card = %+v", vd.Cards[2])
	}
}

func TestViewModelSingle(t *testing.T) {
	vd := viewModel(snap(pods.Status{Model: pods.ModelAirPodsMax, Single: true, Left: pods.Pod{Level: 8}, Right: pods.Pod{Level: 15}, Case: pods.Pod{Level: 15}}, true, false))
	if vd.Title != "AirPods Max" || len(vd.Cards) != 1 {
		t.Fatalf("single vd = %+v", vd)
	}
	if vd.Cards[0].Value != "85%" || !isPNG(vd.Cards[0].Image) {
		t.Errorf("single card = %+v", vd.Cards[0])
	}
}

func TestViewModelDisconnectedPodUsesDimArt(t *testing.T) {
	vd := viewModel(snap(pods.Status{Model: pods.ModelAirPodsPro, Left: pods.Pod{Level: 5}, Right: pods.Pod{Level: 15}, Case: pods.Pod{Level: 8}}, true, false))
	right := vd.Cards[1]
	if right.Value != "—" {
		t.Errorf("disconnected Right Value = %q, want —", right.Value)
	}
	if !isPNG(right.Image) {
		t.Error("disconnected Right should still have (dimmed) artwork")
	}
}

func TestViewModelStale(t *testing.T) {
	vd := viewModel(snap(pods.Status{Model: pods.ModelAirPodsPro}, true, true))
	if len(vd.Cards) != 1 {
		t.Fatalf("stale cards = %+v", vd.Cards)
	}
	if vd.Cards[0].Value != "updating…" {
		t.Errorf("stale card Value = %q, want updating…", vd.Cards[0].Value)
	}
}

// TestBuildContentSmoke ensures the Fyne widget tree builds without panicking
// for the main snapshot shapes (no display needed to construct widgets).
func TestBuildContentSmoke(t *testing.T) {
	cases := []ipc.Snapshot{
		snap(pods.Status{Model: pods.ModelAirPodsPro, Left: pods.Pod{Level: 5}, Right: pods.Pod{Level: 10}, Case: pods.Pod{Level: 8}}, true, false),
		snap(pods.Status{Model: pods.ModelAirPodsMax, Single: true, Left: pods.Pod{Level: 8}}, true, false),
		snap(pods.Status{Model: pods.ModelAirPodsPro}, true, true),
		{},
	}
	for i, s := range cases {
		if obj := buildContent(viewModel(s)); obj == nil {
			t.Errorf("case %d: buildContent returned nil", i)
		}
	}
}
