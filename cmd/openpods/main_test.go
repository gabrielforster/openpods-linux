package main

import (
	"bytes"
	"strings"
	"testing"

	"openpods-linux/pods"
)

func TestFormatHumanStereo(t *testing.T) {
	s := pods.Status{
		Model: pods.ModelAirPodsPro,
		Left:  pods.Pod{Level: 5},
		Right: pods.Pod{Level: 10, Charging: true},
		Case:  pods.Pod{Level: 8},
	}
	out := formatHuman(s)
	for _, want := range []string{"AirPods Pro", "Left", "55%", "Right", "100%", "charging", "Case", "85%"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatHuman output missing %q:\n%s", want, out)
		}
	}
}

func TestFormatHumanSingle(t *testing.T) {
	s := pods.Status{
		Model:  pods.ModelAirPodsMax,
		Single: true,
		Left:   pods.Pod{Level: 8},
		Right:  pods.Pod{Level: 15},
		Case:   pods.Pod{Level: 15},
	}
	out := formatHuman(s)
	if !strings.Contains(out, "AirPods Max") || !strings.Contains(out, "85%") {
		t.Errorf("formatHuman single output unexpected:\n%s", out)
	}
	// A single device must not print Left/Right/Case rows.
	if strings.Contains(out, "Right") || strings.Contains(out, "Case") {
		t.Errorf("single output should not list Right/Case:\n%s", out)
	}
}

func TestFormatHumanDisconnectedPod(t *testing.T) {
	s := pods.Status{
		Model: pods.ModelAirPodsPro,
		Left:  pods.Pod{Level: 5},
		Right: pods.Pod{Level: 15}, // disconnected
		Case:  pods.Pod{Level: 8},
	}
	out := formatHuman(s)
	if !strings.Contains(out, "—") {
		t.Errorf("disconnected pod should render as em dash:\n%s", out)
	}
}

func TestRunStatusReplay(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"status", "--replay", "--timeout", "3s"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "AirPods Pro") || !strings.Contains(out, "55%") {
		t.Errorf("replay status output unexpected:\n%s", out)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"bogus"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown") {
		t.Errorf("stderr should mention unknown command: %q", stderr.String())
	}
}
