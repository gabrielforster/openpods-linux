// Package ipc defines the status snapshot exchanged between the daemon and its
// frontends, plus (later) the Unix-socket server and client. The wire format is
// newline-delimited JSON (NDJSON): one Snapshot object per line.
package ipc

import (
	"encoding/json"
	"time"

	"openpods-linux/pods"
)

// PodView is the JSON view of one earbud or the case. A pod that has no valid
// reading is represented by a nil *PodView (omitted from the JSON) rather than
// a zero value, so consumers never misread "disconnected" as "5%".
type PodView struct {
	Percent  int  `json:"percent"`
	Charging bool `json:"charging"`
	InEar    bool `json:"in_ear,omitempty"` // not meaningful for the case
}

// Snapshot is the daemon's current view of the AirPods, serialized to one NDJSON
// line. For single-figure models only Left is populated (with Single=true). When
// Stale is true the battery figures are hidden (all pods nil) while Connected may
// still hold, mirroring the Android "updating" state.
type Snapshot struct {
	Connected bool      `json:"connected"`
	Stale     bool      `json:"stale"`
	Model     string    `json:"model"`
	Single    bool      `json:"single"`
	Left      *PodView  `json:"left,omitempty"`
	Right     *PodView  `json:"right,omitempty"`
	Case      *PodView  `json:"case,omitempty"`
	Updated   time.Time `json:"updated"`
}

// FromStatus builds a Snapshot from a decoded status plus the daemon's connection
// and staleness flags and the capture time. Disconnected pods and (when stale)
// all pods are omitted.
func FromStatus(s pods.Status, connected, stale bool, updated time.Time) Snapshot {
	snap := Snapshot{
		Connected: connected,
		Stale:     stale,
		Model:     s.Model.String(),
		Single:    s.Single,
		Updated:   updated,
	}
	if stale {
		return snap // figures hidden while the reading is stale
	}
	snap.Left = podView(s.Left)
	if !s.Single {
		snap.Right = podView(s.Right)
		snap.Case = podView(s.Case)
	}
	return snap
}

func podView(p pods.Pod) *PodView {
	pct, ok := p.Percent()
	if !ok {
		return nil
	}
	return &PodView{Percent: pct, Charging: p.Charging, InEar: p.InEar}
}

// Encode marshals a Snapshot to a single newline-terminated NDJSON line.
func Encode(s Snapshot) ([]byte, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Decode parses one NDJSON line into a Snapshot.
func Decode(line []byte) (Snapshot, error) {
	var s Snapshot
	err := json.Unmarshal(line, &s)
	return s, err
}
