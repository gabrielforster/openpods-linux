// Package core holds the daemon's authoritative state: the latest decoded
// status, the AirPods connection flag, and the freshness (staleness) rule. It
// turns the scanner's event streams into a stream of status snapshots plus
// connect/disconnect notification edges.
package core

import (
	"log/slog"
	"sync"
	"time"

	"openpods-linux/ble"
	"openpods-linux/ipc"
	"openpods-linux/pods"
)

// Defaults ported from the Android NotificationThread/NotificationBuilder:
// poll every second, hide figures when the last beacon is older than 30s.
const (
	DefaultStaleAfter   = 30 * time.Second
	DefaultPollInterval = time.Second
)

// state is the pure, clock-driven core state (no goroutines). It mirrors the
// Android service's mStatus + mMaybeConnected plus the staleness timeout.
type state struct {
	status     pods.Status
	haveStatus bool
	connected  bool
	lastBeacon time.Time
	staleAfter time.Duration
}

func newState(staleAfter time.Duration) *state {
	return &state{staleAfter: staleAfter}
}

func (st *state) onStatus(s pods.Status, now time.Time) {
	st.status = s
	st.haveStatus = true
	st.lastBeacon = now
}

// setConnected updates the connection flag and reports whether it changed.
func (st *state) setConnected(c bool) bool {
	if st.connected == c {
		return false
	}
	st.connected = c
	return true
}

// stale reports whether the battery figures should be hidden: either no beacon
// has arrived yet, or the last one is older than the staleness window.
func (st *state) stale(now time.Time) bool {
	return !st.haveStatus || now.Sub(st.lastBeacon) > st.staleAfter
}

func (st *state) snapshot(now time.Time) ipc.Snapshot {
	return ipc.FromStatus(st.status, st.connected, st.stale(now), st.lastBeacon)
}

// NotificationKind identifies a meaningful transition worth surfacing.
type NotificationKind int

const (
	NotifyConnected NotificationKind = iota
	NotifyDisconnected
)

func (k NotificationKind) String() string {
	switch k {
	case NotifyConnected:
		return "connected"
	case NotifyDisconnected:
		return "disconnected"
	default:
		return "unknown"
	}
}

// Notification is an edge the daemon may surface to the user.
type Notification struct {
	Kind     NotificationKind
	Snapshot ipc.Snapshot
}

// Monitor consumes a Scanner and exposes the current status snapshot, a stream
// of snapshot changes (including staleness transitions), and connect/disconnect
// notifications.
type Monitor struct {
	sc           ble.Scanner
	st           *state
	pollInterval time.Duration
	now          func() time.Time

	snapshots chan ipc.Snapshot
	notes     chan Notification

	mu      sync.Mutex
	current ipc.Snapshot

	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

// NewMonitor starts consuming sc in the background. staleAfter is the freshness
// window; pollInterval is how often staleness is re-evaluated. Call Close to
// stop.
func NewMonitor(sc ble.Scanner, staleAfter, pollInterval time.Duration) *Monitor {
	m := &Monitor{
		sc:           sc,
		st:           newState(staleAfter),
		pollInterval: pollInterval,
		now:          time.Now,
		snapshots:    make(chan ipc.Snapshot, 1), // latest-wins
		notes:        make(chan Notification, 8),
		done:         make(chan struct{}),
	}
	m.current = m.st.snapshot(m.now()) // meaningful before any event arrives
	m.wg.Add(1)
	go m.run()
	return m
}

func (m *Monitor) Snapshots() <-chan ipc.Snapshot     { return m.snapshots }
func (m *Monitor) Notifications() <-chan Notification { return m.notes }

// Current returns the most recent snapshot (safe for new IPC clients to read).
func (m *Monitor) Current() ipc.Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

func (m *Monitor) Close() error {
	m.closeOnce.Do(func() { close(m.done) })
	err := m.sc.Close()
	m.wg.Wait()
	return err
}

func (m *Monitor) run() {
	defer m.wg.Done()
	defer close(m.snapshots)
	defer close(m.notes)

	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	updates := m.sc.Updates()
	conns := m.sc.Connected()
	lastStale := m.st.stale(m.now())

	emit := func() {
		snap := m.st.snapshot(m.now())
		m.mu.Lock()
		m.current = snap
		m.mu.Unlock()
		pushLatest(m.snapshots, snap)
		lastStale = snap.Stale
	}

	for {
		select {
		case <-m.done:
			return

		case s, ok := <-updates:
			if !ok {
				return
			}
			m.st.onStatus(s, m.now())
			emit()

		case c, ok := <-conns:
			if !ok {
				return
			}
			if m.st.setConnected(c) {
				emit()
				kind := NotifyConnected
				if !c {
					kind = NotifyDisconnected
				}
				m.notify(Notification{Kind: kind, Snapshot: m.Current()})
			}

		case <-ticker.C:
			// Emit only when the staleness flag actually flips.
			if m.st.stale(m.now()) != lastStale {
				emit()
			}
		}
	}
}

func (m *Monitor) notify(n Notification) {
	select {
	case m.notes <- n:
	default:
		slog.Warn("openpods: dropped notification (buffer full)", "kind", n.Kind)
	}
}

// pushLatest is a non-blocking "latest wins" send on a buffered(1) channel.
func pushLatest(ch chan ipc.Snapshot, s ipc.Snapshot) {
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- s:
	default:
	}
}
