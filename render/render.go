// Package render turns a status snapshot into the textual forms the frontends
// share: a multi-line Human view, a one-line labeled Line (notifications), and
// a compact unlabeled summary (status bars, tray titles).
package render

import (
	"fmt"
	"strconv"
	"strings"

	"openpods-linux/ipc"
)

// Human renders a multi-line, human-readable view (model name then per-pod
// lines, or "updating…" when stale).
func Human(s ipc.Snapshot) string {
	var b strings.Builder
	fmt.Fprintln(&b, Name(s))
	switch {
	case s.Stale:
		fmt.Fprintln(&b, "  updating…")
	case s.Single:
		writePod(&b, "Battery", s.Left)
	default:
		writePod(&b, "Left", s.Left)
		writePod(&b, "Right", s.Right)
		writePod(&b, "Case", s.Case)
	}
	return b.String()
}

// Line renders a one-line labeled summary, e.g. "L 55% · R 100%⚡ · Case 85%".
// Single models show the lone figure; a stale snapshot renders empty.
func Line(s ipc.Snapshot) string {
	if s.Stale {
		return ""
	}
	if s.Single {
		return pct(s.Left)
	}
	var parts []string
	for _, lp := range []labeled{{"L", s.Left}, {"R", s.Right}, {"Case", s.Case}} {
		if t := pct(lp.pod); t != "" {
			parts = append(parts, lp.label+" "+t)
		}
	}
	return strings.Join(parts, " · ")
}

// Compact renders a short unlabeled summary, e.g. "55% 100% 85%" (or "…" stale).
func Compact(s ipc.Snapshot) string {
	if s.Stale {
		return "…"
	}
	if s.Single {
		return pct(s.Left)
	}
	var parts []string
	for _, p := range []*ipc.PodView{s.Left, s.Right, s.Case} {
		if t := pct(p); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " ")
}

// Name returns the snapshot's display name, defaulting to "AirPods".
func Name(s ipc.Snapshot) string {
	if s.Name != "" {
		return s.Name
	}
	return "AirPods"
}

type labeled struct {
	label string
	pod   *ipc.PodView
}

func writePod(b *strings.Builder, label string, p *ipc.PodView) {
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

func pct(p *ipc.PodView) string {
	if p == nil {
		return ""
	}
	s := strconv.Itoa(p.Percent) + "%"
	if p.Charging {
		s += "⚡"
	}
	return s
}
