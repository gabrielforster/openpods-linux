// Command openpodsd is the OpenPods daemon. It owns the single BlueZ scan,
// maintains the authoritative status (battery + connection + staleness), serves
// it to frontends over a Unix socket as NDJSON, and fires desktop notifications
// on connect/disconnect. Following the upstream posture, recoverable errors are
// logged and the daemon keeps running.
package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"openpods-linux/ble"
	"openpods-linux/core"
	"openpods-linux/ipc"
	"openpods-linux/notify"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	replay := flag.Bool("replay", false, "use a fake beacon source instead of Bluetooth")
	socket := flag.String("socket", ipc.DefaultSocketPath(), "Unix socket path to serve status on")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	src, err := makeSource(*replay)
	if err != nil {
		slog.Error("openpodsd: cannot start scanner", "err", err)
		os.Exit(1)
	}

	var notifier notify.Notifier
	if n, err := notify.New(); err != nil {
		slog.Warn("openpodsd: notifications unavailable", "err", err)
	} else {
		notifier = n
		defer notifier.Close()
	}

	if err := run(ctx, src, *socket, notifier); err != nil {
		slog.Error("openpodsd: exited with error", "err", err)
		os.Exit(1)
	}
}

// run wires the scanner, state monitor, IPC server, and notifier together and
// pumps status snapshots to clients until ctx is cancelled or the source stops.
func run(ctx context.Context, src ble.Source, socketPath string, notifier notify.Notifier) error {
	sc := ble.NewScanner(src, ble.DefaultMinRSSI, ble.DefaultRecentWindow)
	mon := core.NewMonitor(sc, core.DefaultStaleAfter, core.DefaultPollInterval)
	defer mon.Close()

	srv, err := ipc.NewServer(socketPath)
	if err != nil {
		return fmt.Errorf("openpodsd: %w", err)
	}
	defer srv.Close()

	slog.Info("openpodsd started", "socket", socketPath)
	go forwardNotifications(mon, notifier)

	for {
		select {
		case <-ctx.Done():
			return nil
		case snap, ok := <-mon.Snapshots():
			if !ok {
				return nil // monitor stopped (source closed)
			}
			srv.Broadcast(snap)
		}
	}
}

func forwardNotifications(mon *core.Monitor, notifier notify.Notifier) {
	for n := range mon.Notifications() {
		if notifier == nil {
			continue
		}
		summary, body := notify.Message(n.Kind == core.NotifyConnected, n.Snapshot)
		if err := notifier.Notify(summary, body); err != nil {
			slog.Warn("openpodsd: notification failed", "err", err)
		}
	}
}

func makeSource(replay bool) (ble.Source, error) {
	if replay {
		return ble.NewReplaySource(demoBeacons(), 2*time.Second), nil
	}
	return ble.NewBlueZSource()
}

// demoBeacons is the canned advertisement used by --replay: AirPods Pro at
// Left 55%, Right 100%, Case 85%.
func demoBeacons() []ble.Beacon {
	data, _ := hex.DecodeString("0719010E2020A508" + strings.Repeat("00", 19))
	return []ble.Beacon{{Address: "replay", Data: data, RSSI: -45}}
}
