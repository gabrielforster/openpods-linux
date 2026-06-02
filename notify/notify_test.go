package notify_test

import (
	"strings"
	"testing"
	"time"

	"openpods-linux/ipc"
	"openpods-linux/notify"
	"openpods-linux/pods"
)

func snap(s pods.Status, connected, stale bool) ipc.Snapshot {
	return ipc.FromStatus(s, connected, stale, time.Unix(0, 0))
}

func TestMessageConnectedStereo(t *testing.T) {
	s := pods.Status{Model: pods.ModelAirPodsPro, Left: pods.Pod{Level: 5}, Right: pods.Pod{Level: 10}, Case: pods.Pod{Level: 8}}
	summary, body := notify.Message(true, snap(s, true, false))
	if summary != "AirPods Pro connected" {
		t.Errorf("summary = %q", summary)
	}
	for _, w := range []string{"L 55%", "R 100%", "Case 85%"} {
		if !strings.Contains(body, w) {
			t.Errorf("body %q missing %q", body, w)
		}
	}
}

func TestMessageShowsCharging(t *testing.T) {
	s := pods.Status{Model: pods.ModelAirPodsPro, Left: pods.Pod{Level: 5, Charging: true}, Right: pods.Pod{Level: 10}, Case: pods.Pod{Level: 8}}
	_, body := notify.Message(true, snap(s, true, false))
	if !strings.Contains(body, "⚡") {
		t.Errorf("charging pod should be marked, body = %q", body)
	}
}

func TestMessageSingle(t *testing.T) {
	s := pods.Status{Model: pods.ModelAirPodsMax, Single: true, Left: pods.Pod{Level: 8}, Right: pods.Pod{Level: 15}, Case: pods.Pod{Level: 15}}
	summary, body := notify.Message(true, snap(s, true, false))
	if summary != "AirPods Max connected" {
		t.Errorf("summary = %q", summary)
	}
	if body != "85%" {
		t.Errorf("single body = %q, want 85%%", body)
	}
}

func TestMessageDisconnected(t *testing.T) {
	s := pods.Status{Model: pods.ModelAirPodsPro, Left: pods.Pod{Level: 5}, Right: pods.Pod{Level: 10}, Case: pods.Pod{Level: 8}}
	summary, body := notify.Message(false, snap(s, false, false))
	if !strings.Contains(summary, "disconnected") {
		t.Errorf("summary = %q, want it to mention disconnected", summary)
	}
	if body != "" {
		t.Errorf("disconnect body = %q, want empty", body)
	}
}

func TestMessageStaleHasNoFigures(t *testing.T) {
	s := pods.Status{Model: pods.ModelAirPodsPro, Left: pods.Pod{Level: 5}, Right: pods.Pod{Level: 10}, Case: pods.Pod{Level: 8}}
	_, body := notify.Message(true, snap(s, true, true))
	if body != "" {
		t.Errorf("stale body = %q, want empty (figures hidden)", body)
	}
}
