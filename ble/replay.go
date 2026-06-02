package ble

import (
	"sync"
	"time"
)

// replaySource is a Source that synthesizes beacons from a canned list, for
// developing and demoing frontends without AirPods present (the --replay mode).
// It announces a connected device once, then emits the beacons in a loop,
// re-stamping each with the current time so the selector treats them as fresh.
type replaySource struct {
	beacons  []Beacon
	interval time.Duration
	now      func() time.Time

	out       chan Beacon
	conns     chan ConnEvent
	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

// NewReplaySource returns a Source that emits the given beacons in a loop spaced
// by interval (0 = as fast as the consumer reads). It first emits a single
// "connected" event so connection-gated frontends light up.
func NewReplaySource(beacons []Beacon, interval time.Duration) Source {
	r := &replaySource{
		beacons:  beacons,
		interval: interval,
		now:      time.Now,
		out:      make(chan Beacon),
		conns:    make(chan ConnEvent, 1),
		done:     make(chan struct{}),
	}
	r.wg.Add(1)
	go r.run()
	return r
}

func (r *replaySource) Beacons() <-chan Beacon        { return r.out }
func (r *replaySource) Connections() <-chan ConnEvent { return r.conns }

func (r *replaySource) Close() error {
	r.closeOnce.Do(func() { close(r.done) })
	r.wg.Wait()
	return nil
}

func (r *replaySource) run() {
	defer r.wg.Done()
	defer close(r.out)
	defer close(r.conns)

	// conns is buffered(1); this never blocks.
	r.conns <- ConnEvent{Address: "replay", Connected: true}

	if len(r.beacons) == 0 {
		<-r.done
		return
	}

	for i := 0; ; i++ {
		b := r.beacons[i%len(r.beacons)]
		b.Time = r.now()
		select {
		case r.out <- b:
		case <-r.done:
			return
		}
		if r.interval > 0 {
			select {
			case <-time.After(r.interval):
			case <-r.done:
				return
			}
		}
	}
}
