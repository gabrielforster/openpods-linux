package render_test

import (
	"strings"
	"testing"
	"time"

	"openpods-linux/ipc"
	"openpods-linux/pods"
	"openpods-linux/render"
)

func snap(s pods.Status, connected, stale bool) ipc.Snapshot {
	return ipc.FromStatus(s, connected, stale, time.Unix(0, 0))
}

func pro(charging bool) ipc.Snapshot {
	return snap(pods.Status{
		Model: pods.ModelAirPodsPro,
		Left:  pods.Pod{Level: 5},
		Right: pods.Pod{Level: 10, Charging: charging},
		Case:  pods.Pod{Level: 8},
	}, true, false)
}

func maxSingle() ipc.Snapshot {
	return snap(pods.Status{Model: pods.ModelAirPodsMax, Single: true, Left: pods.Pod{Level: 8}, Right: pods.Pod{Level: 15}, Case: pods.Pod{Level: 15}}, true, false)
}

func TestHuman(t *testing.T) {
	out := render.Human(pro(true))
	for _, w := range []string{"AirPods Pro", "Left", "55%", "Right", "100%", "charging", "Case", "85%"} {
		if !strings.Contains(out, w) {
			t.Errorf("Human missing %q:\n%s", w, out)
		}
	}

	if s := render.Human(maxSingle()); !strings.Contains(s, "AirPods Max") || !strings.Contains(s, "85%") || strings.Contains(s, "Right") {
		t.Errorf("single Human unexpected:\n%s", s)
	}

	if s := render.Human(snap(pods.Status{Model: pods.ModelAirPodsPro}, true, true)); !strings.Contains(strings.ToLower(s), "updating") {
		t.Errorf("stale Human should say updating:\n%s", s)
	}

	dp := snap(pods.Status{Model: pods.ModelAirPodsPro, Left: pods.Pod{Level: 5}, Right: pods.Pod{Level: 15}, Case: pods.Pod{Level: 8}}, true, false)
	if s := render.Human(dp); !strings.Contains(s, "—") {
		t.Errorf("disconnected pod should render as em dash:\n%s", s)
	}
}

func TestLine(t *testing.T) {
	if got := render.Line(pro(false)); got != "L 55% · R 100% · Case 85%" {
		t.Errorf("Line = %q", got)
	}
	if got := render.Line(pro(true)); !strings.Contains(got, "⚡") {
		t.Errorf("Line should mark charging: %q", got)
	}
	if got := render.Line(maxSingle()); got != "85%" {
		t.Errorf("single Line = %q, want 85%%", got)
	}
	if got := render.Line(snap(pods.Status{Model: pods.ModelAirPodsPro}, true, true)); got != "" {
		t.Errorf("stale Line = %q, want empty", got)
	}
}

func TestCompact(t *testing.T) {
	if got := render.Compact(pro(false)); got != "55% 100% 85%" {
		t.Errorf("Compact = %q", got)
	}
	if got := render.Compact(maxSingle()); got != "85%" {
		t.Errorf("single Compact = %q", got)
	}
	if got := render.Compact(snap(pods.Status{Model: pods.ModelAirPodsPro}, true, true)); got != "…" {
		t.Errorf("stale Compact = %q, want …", got)
	}
}
