// Command openpods is the CLI frontend. `openpods status` prints the current
// AirPods/Beats battery: it reads the daemon's socket when one is running, and
// otherwise falls back to a bounded one-shot BLE scan. Output can be human
// readable (default), raw JSON (--json), or waybar JSON (--waybar); --watch
// streams updates from the daemon.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"openpods-linux/ble"
	"openpods-linux/ipc"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

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
	timeout := fs.Duration("timeout", 10*time.Second, "how long the one-shot scan waits for a beacon")
	asJSON := fs.Bool("json", false, "print the raw status JSON")
	waybar := fs.Bool("waybar", false, "print waybar JSON (text/tooltip/class)")
	watch := fs.Bool("watch", false, "stream updates from the daemon (requires the daemon)")
	socket := fs.String("socket", ipc.DefaultSocketPath(), "daemon socket path")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	format := formatHuman
	switch {
	case *waybar:
		format = formatWaybar
	case *asJSON:
		format = formatJSON
	}

	// Prefer the daemon when one is running (unless --replay forces the fake source).
	if !*replay {
		if cl, err := ipc.Dial(*socket); err == nil {
			defer cl.Close()
			return streamFromDaemon(cl, format, *watch, stdout, stderr)
		}
	}

	if *watch {
		fmt.Fprintln(stderr, "openpods: --watch needs the daemon; doing a one-shot scan instead")
	}
	return oneShot(*replay, *timeout, format, stdout, stderr)
}

type formatter func(ipc.Snapshot) string

func streamFromDaemon(cl *ipc.Client, format formatter, watch bool, stdout, stderr io.Writer) int {
	for {
		snap, err := cl.Read()
		if err != nil {
			fmt.Fprintf(stderr, "openpods: daemon connection closed: %v\n", err)
			return 1
		}
		fmt.Fprint(stdout, format(snap))
		if !watch {
			return 0
		}
	}
}

func oneShot(replay bool, timeout time.Duration, format formatter, stdout, stderr io.Writer) int {
	src, err := newSource(replay)
	if err != nil {
		fmt.Fprintf(stderr, "openpods: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	st, ok := ble.Scan(ctx, src, ble.DefaultMinRSSI, ble.DefaultRecentWindow)
	if !ok {
		fmt.Fprintln(stderr, "openpods: no AirPods found (out of the case and nearby?)")
		return 1
	}
	// A one-shot scan that found a beacon implies a present, connected device.
	fmt.Fprint(stdout, format(ipc.FromStatus(st, true, false, time.Now())))
	return 0
}

func newSource(replay bool) (ble.Source, error) {
	if replay {
		return ble.NewReplaySource(sampleBeacons(), 2*time.Second), nil
	}
	return ble.NewBlueZSource()
}

func sampleBeacons() []ble.Beacon {
	data, _ := hex.DecodeString("0719010E2020A508" + strings.Repeat("00", 19))
	return []ble.Beacon{{Address: "replay", Data: data, RSSI: -45}}
}

// --- formatters ---

func formatHuman(s ipc.Snapshot) string {
	var b strings.Builder
	fmt.Fprintln(&b, nameOf(s))
	switch {
	case s.Stale:
		fmt.Fprintln(&b, "  updating…")
	case s.Single:
		writePodView(&b, "Battery", s.Left)
	default:
		writePodView(&b, "Left", s.Left)
		writePodView(&b, "Right", s.Right)
		writePodView(&b, "Case", s.Case)
	}
	return b.String()
}

func writePodView(b *strings.Builder, label string, p *ipc.PodView) {
	if p == nil {
		fmt.Fprintf(b, "  %-8s —\n", label)
		return
	}
	var extras []string
	if p.Charging {
		extras = append(extras, "charging")
	}
	if p.InEar {
		extras = append(extras, "in ear")
	}
	suffix := ""
	if len(extras) > 0 {
		suffix = "  (" + strings.Join(extras, ", ") + ")"
	}
	fmt.Fprintf(b, "  %-8s %3d%%%s\n", label, p.Percent, suffix)
}

func formatJSON(s ipc.Snapshot) string {
	line, err := ipc.Encode(s)
	if err != nil {
		return "{}\n"
	}
	return string(line)
}

func formatWaybar(s ipc.Snapshot) string {
	out := struct {
		Text    string `json:"text"`
		Tooltip string `json:"tooltip"`
		Class   string `json:"class"`
	}{
		Text:    waybarText(s),
		Tooltip: strings.TrimRight(formatHuman(s), "\n"),
		Class:   waybarClass(s),
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "{}\n"
	}
	return string(b) + "\n"
}

func waybarClass(s ipc.Snapshot) string {
	switch {
	case s.Stale:
		return "stale"
	case s.Connected:
		return "connected"
	default:
		return "disconnected"
	}
}

func waybarText(s ipc.Snapshot) string {
	if s.Stale {
		return "…"
	}
	if s.Single {
		return podPct(s.Left)
	}
	var parts []string
	for _, p := range []*ipc.PodView{s.Left, s.Right, s.Case} {
		if t := podPct(p); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " ")
}

func podPct(p *ipc.PodView) string {
	if p == nil {
		return ""
	}
	s := strconv.Itoa(p.Percent) + "%"
	if p.Charging {
		s += "⚡"
	}
	return s
}

func nameOf(s ipc.Snapshot) string {
	if s.Name != "" {
		return s.Name
	}
	return "AirPods"
}
