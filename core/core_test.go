package core

import (
	"testing"
	"time"

	"openpods-linux/ipc"
	"openpods-linux/pods"
)

var t0 = time.Unix(1_700_000_000, 0)

func airpodsPro() pods.Status {
	return pods.Status{
		Model: pods.ModelAirPodsPro,
		Left:  pods.Pod{Level: 5},
		Right: pods.Pod{Level: 10},
		Case:  pods.Pod{Level: 8},
	}
}

// --- pure state ---

func TestStateStaleness(t *testing.T) {
	st := newState(30 * time.Second)
	if !st.stale(t0) {
		t.Error("should be stale before the first beacon")
	}
	st.onStatus(airpodsPro(), t0)
	if st.stale(t0) {
		t.Error("should be fresh right after a beacon")
	}
	if st.stale(t0.Add(29 * time.Second)) {
		t.Error("should still be fresh at 29s")
	}
	if !st.stale(t0.Add(31 * time.Second)) {
		t.Error("should be stale at 31s")
	}
}

func TestStateConnectedEdge(t *testing.T) {
	st := newState(30 * time.Second)
	if !st.setConnected(true) {
		t.Error("false->true should report a change")
	}
	if st.setConnected(true) {
		t.Error("true->true should not report a change")
	}
	if !st.setConnected(false) {
		t.Error("true->false should report a change")
	}
}

func TestStateSnapshot(t *testing.T) {
	st := newState(30 * time.Second)
	st.onStatus(airpodsPro(), t0)
	st.setConnected(true)
	snap := st.snapshot(t0)
	if !snap.Connected || snap.Stale || snap.Model != "airpodspro" {
		t.Errorf("snapshot header = %+v", snap)
	}
	if snap.Left == nil || snap.Left.Percent != 55 {
		t.Errorf("Left = %+v", snap.Left)
	}
	if !snap.Updated.Equal(t0) {
		t.Errorf("Updated = %v, want %v", snap.Updated, t0)
	}
}

// --- Monitor (goroutine wiring) ---

type fakeScanner struct {
	updates chan pods.Status
	conns   chan bool
}

func newFakeScanner() *fakeScanner {
	return &fakeScanner{updates: make(chan pods.Status), conns: make(chan bool)}
}

func (f *fakeScanner) Updates() <-chan pods.Status { return f.updates }
func (f *fakeScanner) Connected() <-chan bool      { return f.conns }
func (f *fakeScanner) Close() error                { return nil }

func readSnapshot(t *testing.T, ch <-chan ipc.Snapshot) ipc.Snapshot {
	t.Helper()
	select {
	case s := <-ch:
		return s
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a snapshot")
		return ipc.Snapshot{}
	}
}

func TestMonitorEmitsSnapshotOnStatus(t *testing.T) {
	fs := newFakeScanner()
	m := NewMonitor(fs, 30*time.Second, time.Second)
	defer m.Close()

	fs.updates <- airpodsPro()
	snap := readSnapshot(t, m.Snapshots())
	if snap.Model != "airpodspro" || snap.Stale {
		t.Errorf("snapshot = %+v", snap)
	}
	if snap.Left == nil || snap.Left.Percent != 55 {
		t.Errorf("Left = %+v", snap.Left)
	}
}

func TestMonitorConnectionNotifications(t *testing.T) {
	fs := newFakeScanner()
	m := NewMonitor(fs, 30*time.Second, time.Second)
	defer m.Close()

	fs.conns <- true
	select {
	case n := <-m.Notifications():
		if n.Kind != NotifyConnected {
			t.Errorf("Kind = %v, want NotifyConnected", n.Kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no connect notification")
	}

	fs.conns <- false
	select {
	case n := <-m.Notifications():
		if n.Kind != NotifyDisconnected {
			t.Errorf("Kind = %v, want NotifyDisconnected", n.Kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no disconnect notification")
	}
}

func TestMonitorGoesStale(t *testing.T) {
	fs := newFakeScanner()
	// Short windows so the staleness transition happens quickly.
	m := NewMonitor(fs, 40*time.Millisecond, 10*time.Millisecond)
	defer m.Close()

	fs.updates <- airpodsPro()
	if snap := readSnapshot(t, m.Snapshots()); snap.Stale {
		t.Fatal("first snapshot should be fresh")
	}

	// With no further beacons, a later snapshot must flip to stale.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case snap := <-m.Snapshots():
			if snap.Stale {
				return // success
			}
		case <-deadline:
			t.Fatal("never became stale")
		}
	}
}

func TestNotificationKindString(t *testing.T) {
	cases := map[NotificationKind]string{
		NotifyConnected:     "connected",
		NotifyDisconnected:  "disconnected",
		NotificationKind(9): "unknown",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("NotificationKind(%d).String() = %q, want %q", k, got, want)
		}
	}
}

func TestMonitorStopsWhenUpdatesClose(t *testing.T) {
	fs := newFakeScanner()
	m := NewMonitor(fs, 30*time.Second, time.Second)
	defer m.Close()
	close(fs.updates)
	assertSnapshotsClosed(t, m)
}

func TestMonitorStopsWhenConnsClose(t *testing.T) {
	fs := newFakeScanner()
	m := NewMonitor(fs, 30*time.Second, time.Second)
	defer m.Close()
	close(fs.conns)
	assertSnapshotsClosed(t, m)
}

func assertSnapshotsClosed(t *testing.T, m *Monitor) {
	t.Helper()
	select {
	case _, ok := <-m.Snapshots():
		if ok {
			t.Fatal("expected Snapshots to close when the scanner stops")
		}
	case <-time.After(time.Second):
		t.Fatal("Monitor did not stop when the scanner closed")
	}
}

func TestMonitorCurrent(t *testing.T) {
	fs := newFakeScanner()
	m := NewMonitor(fs, 30*time.Second, time.Second)
	defer m.Close()

	fs.updates <- airpodsPro()
	_ = readSnapshot(t, m.Snapshots()) // ensure the emit happened
	if cur := m.Current(); cur.Model != "airpodspro" {
		t.Errorf("Current().Model = %q, want airpodspro", cur.Model)
	}
}
