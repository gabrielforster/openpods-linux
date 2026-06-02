package ipc_test

import (
	"encoding/json"
	"testing"
	"time"

	"openpods-linux/ipc"
	"openpods-linux/pods"
)

var snapTime = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func TestFromStatusStereo(t *testing.T) {
	s := pods.Status{
		Model: pods.ModelAirPodsPro,
		Left:  pods.Pod{Level: 5},
		Right: pods.Pod{Level: 10, Charging: true},
		Case:  pods.Pod{Level: 8},
	}
	snap := ipc.FromStatus(s, true, false, snapTime)

	if !snap.Connected || snap.Stale || snap.Model != "airpodspro" || snap.Single {
		t.Errorf("snapshot header = %+v", snap)
	}
	if snap.Left == nil || snap.Left.Percent != 55 || snap.Left.Charging {
		t.Errorf("Left = %+v", snap.Left)
	}
	if snap.Right == nil || snap.Right.Percent != 100 || !snap.Right.Charging {
		t.Errorf("Right = %+v", snap.Right)
	}
	if snap.Case == nil || snap.Case.Percent != 85 {
		t.Errorf("Case = %+v", snap.Case)
	}
	if !snap.Updated.Equal(snapTime) {
		t.Errorf("Updated = %v, want %v", snap.Updated, snapTime)
	}
}

func TestFromStatusSingle(t *testing.T) {
	s := pods.Status{
		Model:  pods.ModelAirPodsMax,
		Single: true,
		Left:   pods.Pod{Level: 8},
		Right:  pods.Pod{Level: 15},
		Case:   pods.Pod{Level: 15},
	}
	snap := ipc.FromStatus(s, true, false, snapTime)
	if !snap.Single || snap.Left == nil || snap.Left.Percent != 85 {
		t.Errorf("single Left = %+v", snap.Left)
	}
	if snap.Right != nil || snap.Case != nil {
		t.Errorf("single should omit Right/Case, got Right=%+v Case=%+v", snap.Right, snap.Case)
	}
}

func TestFromStatusDisconnectedPodOmitted(t *testing.T) {
	s := pods.Status{
		Model: pods.ModelAirPodsPro,
		Left:  pods.Pod{Level: 5},
		Right: pods.Pod{Level: 15}, // disconnected
		Case:  pods.Pod{Level: 8},
	}
	snap := ipc.FromStatus(s, true, false, snapTime)
	if snap.Left == nil || snap.Right != nil || snap.Case == nil {
		t.Errorf("expected Right omitted, got Left=%+v Right=%+v Case=%+v", snap.Left, snap.Right, snap.Case)
	}
}

func TestFromStatusStaleHidesFigures(t *testing.T) {
	s := pods.Status{
		Model: pods.ModelAirPodsPro,
		Left:  pods.Pod{Level: 5},
		Right: pods.Pod{Level: 10},
		Case:  pods.Pod{Level: 8},
	}
	snap := ipc.FromStatus(s, true, true, snapTime)
	if !snap.Stale {
		t.Error("Stale should be true")
	}
	if snap.Left != nil || snap.Right != nil || snap.Case != nil {
		t.Errorf("stale snapshot should hide figures, got %+v", snap)
	}
	if !snap.Connected {
		t.Error("Connected should be retained while stale")
	}
}

func TestSnapshotJSONFieldNames(t *testing.T) {
	s := pods.Status{
		Model: pods.ModelAirPodsPro,
		Left:  pods.Pod{Level: 8, InEar: true},
		Right: pods.Pod{Level: 9},
		Case:  pods.Pod{Level: 5, Charging: true},
	}
	snap := ipc.FromStatus(s, true, false, snapTime)
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"connected", "stale", "model", "single", "left", "right", "case", "updated"} {
		if _, ok := m[key]; !ok {
			t.Errorf("JSON missing key %q: %s", key, b)
		}
	}
	left := m["left"].(map[string]any)
	if _, ok := left["in_ear"]; !ok {
		t.Errorf("left should include in_ear when true: %s", b)
	}
	// Case has no in-ear concept; the field is omitted.
	if _, ok := m["case"].(map[string]any)["in_ear"]; ok {
		t.Errorf("case should omit in_ear: %s", b)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	s := pods.Status{Model: pods.ModelAirPodsPro, Left: pods.Pod{Level: 5}, Right: pods.Pod{Level: 10}, Case: pods.Pod{Level: 8}}
	snap := ipc.FromStatus(s, true, false, snapTime)

	line, err := ipc.Encode(snap)
	if err != nil {
		t.Fatal(err)
	}
	if line[len(line)-1] != '\n' {
		t.Error("Encode should produce a newline-terminated line")
	}
	got, err := ipc.Decode(line)
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != snap.Model || got.Left.Percent != 55 || !got.Updated.Equal(snapTime) {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestDecodeRejectsMalformedLine(t *testing.T) {
	if _, err := ipc.Decode([]byte("{not json")); err == nil {
		t.Error("Decode should return an error for a malformed line")
	}
}
