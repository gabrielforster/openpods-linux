// Command openpods is the CLI frontend. For now it implements `openpods status`:
// a bounded one-shot BLE scan that prints the current AirPods/Beats battery in
// human-readable form. A --replay flag drives the output from a fake beacon
// source so the command works without Bluetooth.
package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"openpods-linux/ble"
	"openpods-linux/pods"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run dispatches a subcommand and returns the process exit code. It takes its
// output writers so it can be tested without touching the real stdio.
func run(args []string, stdout, stderr io.Writer) int {
	cmd := "status"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}

	switch cmd {
	case "status":
		return cmdStatus(args, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "openpods: unknown command %q (try \"openpods status\")\n", cmd)
		return 2
	}
}

func cmdStatus(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("openpods status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	replay := fs.Bool("replay", false, "use a fake beacon source instead of Bluetooth (no hardware needed)")
	timeout := fs.Duration("timeout", 10*time.Second, "how long to scan before giving up")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	src, err := newSource(*replay)
	if err != nil {
		fmt.Fprintf(stderr, "openpods: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// Scan owns the source and closes it when it returns.
	st, ok := ble.Scan(ctx, src, ble.DefaultMinRSSI, ble.DefaultRecentWindow)
	if !ok {
		fmt.Fprintln(stderr, "openpods: no AirPods found (out of the case and nearby?)")
		return 1
	}

	fmt.Fprint(stdout, formatHuman(st))
	return 0
}

func newSource(replay bool) (ble.Source, error) {
	if replay {
		return ble.NewReplaySource(sampleBeacons(), 2*time.Second), nil
	}
	return ble.NewBlueZSource()
}

// sampleBeacons is the canned advertisement used by --replay: an AirPods Pro at
// Left 55%, Right 100%, Case 85%.
func sampleBeacons() []ble.Beacon {
	data, _ := hex.DecodeString("0719010E2020A508" + strings.Repeat("00", 19))
	return []ble.Beacon{{Address: "replay", Data: data, RSSI: -45}}
}

func formatHuman(s pods.Status) string {
	var b strings.Builder
	fmt.Fprintln(&b, pods.DisplayName(s.Model))
	if s.Single {
		writePod(&b, "Battery", s.Left, false)
	} else {
		writePod(&b, "Left", s.Left, true)
		writePod(&b, "Right", s.Right, true)
		writePod(&b, "Case", s.Case, false)
	}
	return b.String()
}

func writePod(b *strings.Builder, label string, p pods.Pod, showInEar bool) {
	pct, ok := p.Percent()
	if !ok {
		fmt.Fprintf(b, "  %-8s —\n", label)
		return
	}
	var extras []string
	if p.Charging {
		extras = append(extras, "charging")
	}
	if showInEar && p.InEar {
		extras = append(extras, "in ear")
	}
	suffix := ""
	if len(extras) > 0 {
		suffix = "  (" + strings.Join(extras, ", ") + ")"
	}
	fmt.Fprintf(b, "  %-8s %3d%%%s\n", label, pct, suffix)
}
