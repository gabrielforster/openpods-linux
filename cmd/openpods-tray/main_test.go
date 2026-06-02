package main

import (
	"strings"
	"testing"
	"time"

	"openpods-linux/ipc"
	"openpods-linux/pods"
)

type fakeUI struct {
	title, tooltip string
}

func (f *fakeUI) SetTitle(s string)   { f.title = s }
func (f *fakeUI) SetTooltip(s string) { f.tooltip = s }

func TestApplySnapshotShowsBattery(t *testing.T) {
	snap := ipc.FromStatus(pods.Status{
		Model: pods.ModelAirPodsPro,
		Left:  pods.Pod{Level: 5},
		Right: pods.Pod{Level: 10},
		Case:  pods.Pod{Level: 8},
	}, true, false, time.Unix(0, 0))

	ui := &fakeUI{}
	applySnapshot(ui, snap)
	if ui.title != "55% 100% 85%" {
		t.Errorf("title = %q, want \"55%% 100%% 85%%\"", ui.title)
	}
	if !strings.Contains(ui.tooltip, "AirPods Pro") {
		t.Errorf("tooltip = %q, want it to include the model name", ui.tooltip)
	}
}

func TestApplySnapshotStale(t *testing.T) {
	snap := ipc.FromStatus(pods.Status{Model: pods.ModelAirPodsPro}, true, true, time.Unix(0, 0))
	ui := &fakeUI{}
	applySnapshot(ui, snap)
	if ui.title != "…" {
		t.Errorf("stale title = %q, want …", ui.title)
	}
}
