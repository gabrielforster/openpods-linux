package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"openpods-linux/ble"
	"openpods-linux/ipc"
)

// TestDaemonServesReplaySnapshots drives the whole daemon pipeline offline:
// replay source -> scanner -> core.Monitor -> ipc.Server -> client.
func TestDaemonServesReplaySnapshots(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "openpods.sock")
	src := ble.NewReplaySource(demoBeacons(), 20*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errc := make(chan error, 1)
	go func() { errc <- run(ctx, src, socket, nil) }()

	cl := dialWithRetry(t, socket)
	defer cl.Close()

	snaps := make(chan ipc.Snapshot)
	go func() {
		for {
			s, err := cl.Read()
			if err != nil {
				close(snaps)
				return
			}
			snaps <- s
		}
	}()

	deadline := time.After(3 * time.Second)
	for {
		select {
		case s, ok := <-snaps:
			if !ok {
				t.Fatal("client stream closed before an AirPods Pro snapshot")
			}
			if s.Model == "airpodspro" {
				if s.Left == nil || s.Left.Percent != 55 {
					t.Errorf("Left = %+v, want 55%%", s.Left)
				}
				cancel()
				if err := <-errc; err != nil {
					t.Errorf("run returned error: %v", err)
				}
				return
			}
		case <-deadline:
			t.Fatal("never received a decoded AirPods Pro snapshot")
		}
	}
}

func dialWithRetry(t *testing.T, socket string) *ipc.Client {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		cl, err := ipc.Dial(socket)
		if err == nil {
			return cl
		}
		if time.Now().After(deadline) {
			t.Fatalf("dial %s: %v", socket, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
