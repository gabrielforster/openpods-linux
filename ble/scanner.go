package ble

import (
	"context"
	"sync"
	"time"

	"openpods-linux/pods"
)

// Scanner consumes a Source, applies the strongest-recent + RSSI heuristic,
// decodes the chosen beacon, and exposes the latest decoded status plus the
// AirPods connection state. It mirrors the Android PodsService wiring: status
// updates and the "maybe connected" flag are tracked independently.
type Scanner interface {
	// Updates delivers decoded beacons that cleared the heuristic. The channel
	// keeps only the most recent value (a slow consumer never stalls scanning).
	Updates() <-chan pods.Status
	// Connected delivers the aggregate AirPods audio-link state on change.
	Connected() <-chan bool
	Close() error
}

type scanner struct {
	src     Source
	sel     *selector
	updates chan pods.Status
	conns   chan bool

	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

// NewScanner starts consuming src in the background and returns a Scanner.
// Call Close to stop it.
func NewScanner(src Source, minRSSI int16, window time.Duration) Scanner {
	s := &scanner{
		src:     src,
		sel:     newSelector(minRSSI, window),
		updates: make(chan pods.Status, 1),
		conns:   make(chan bool, 1),
		done:    make(chan struct{}),
	}
	s.wg.Add(1)
	go s.run()
	return s
}

func (s *scanner) Updates() <-chan pods.Status { return s.updates }
func (s *scanner) Connected() <-chan bool      { return s.conns }

func (s *scanner) Close() error {
	s.closeOnce.Do(func() { close(s.done) })
	err := s.src.Close()
	s.wg.Wait()
	return err
}

func (s *scanner) run() {
	defer s.wg.Done()
	// Closing the outputs when we stop signals consumers that the scanner is
	// done. Only run emits, and it never emits after this point, so the
	// non-blocking sends in emit can't race with these closes.
	defer close(s.updates)
	defer close(s.conns)

	connected := make(map[string]bool) // addresses of currently-connected AirPods
	aggregate := false

	beacons := s.src.Beacons()
	conns := s.src.Connections()

	for {
		select {
		case <-s.done:
			return

		case b, ok := <-beacons:
			if !ok {
				return // source closed
			}
			best, pass := s.sel.best(b)
			if !pass {
				continue
			}
			st, err := pods.Decode(best.Data)
			if err != nil {
				continue // malformed payload; skip silently
			}
			emit(s.updates, st)

		case c, ok := <-conns:
			if !ok {
				return // source closed
			}
			if c.Connected {
				connected[c.Address] = true
			} else {
				delete(connected, c.Address)
			}
			if next := len(connected) > 0; next != aggregate {
				aggregate = next
				emit(s.conns, next)
			}
		}
	}
}

// emit performs a non-blocking "latest wins" send on a buffered(1) channel:
// drop any stale unread value, then enqueue v. The scanner is the sole producer.
func emit[T any](ch chan T, v T) {
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- v:
	default:
	}
}

// Scan runs a bounded one-shot scan: it returns the first decoded status that
// clears the heuristic, or (zero, false) if ctx is done first. Used by the CLI
// when no daemon is available.
//
// Returning the first qualifying beacon keeps the command responsive; the
// RSSI >= -60 gate already filters out distant devices. The daemon's streaming
// Scanner continuously refines the choice via the strongest-recent heuristic.
func Scan(ctx context.Context, src Source, minRSSI int16, window time.Duration) (pods.Status, bool) {
	sc := NewScanner(src, minRSSI, window)
	defer sc.Close()

	select {
	case st, ok := <-sc.Updates():
		if !ok {
			return pods.Status{}, false // scanner stopped before any status
		}
		return st, true
	case <-ctx.Done():
		return pods.Status{}, false
	}
}
