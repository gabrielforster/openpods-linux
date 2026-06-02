package main

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"openpods-linux/ipc"
	"openpods-linux/pods"
)

func snapPro(charging bool) ipc.Snapshot {
	s := pods.Status{
		Model: pods.ModelAirPodsPro,
		Left:  pods.Pod{Level: 5},
		Right: pods.Pod{Level: 10, Charging: charging},
		Case:  pods.Pod{Level: 8},
	}
	return ipc.FromStatus(s, true, false, time.Unix(0, 0))
}

func tempSocket(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "openpods.sock")
}

func TestFormatHumanStereo(t *testing.T) {
	out := formatHuman(snapPro(true))
	for _, want := range []string{"AirPods Pro", "Left", "55%", "Right", "100%", "charging", "Case", "85%"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatHuman missing %q:\n%s", want, out)
		}
	}
}

func TestFormatHumanSingle(t *testing.T) {
	s := pods.Status{Model: pods.ModelAirPodsMax, Single: true, Left: pods.Pod{Level: 8}, Right: pods.Pod{Level: 15}, Case: pods.Pod{Level: 15}}
	out := formatHuman(ipc.FromStatus(s, true, false, time.Unix(0, 0)))
	if !strings.Contains(out, "AirPods Max") || !strings.Contains(out, "85%") {
		t.Errorf("single output unexpected:\n%s", out)
	}
	if strings.Contains(out, "Right") || strings.Contains(out, "Case") {
		t.Errorf("single output should not list Right/Case:\n%s", out)
	}
}

func TestFormatHumanStale(t *testing.T) {
	s := pods.Status{Model: pods.ModelAirPodsPro, Left: pods.Pod{Level: 5}, Right: pods.Pod{Level: 10}, Case: pods.Pod{Level: 8}}
	out := formatHuman(ipc.FromStatus(s, true, true, time.Unix(0, 0)))
	if !strings.Contains(strings.ToLower(out), "updating") {
		t.Errorf("stale output should say updating:\n%s", out)
	}
}

func TestFormatHumanDisconnectedPod(t *testing.T) {
	s := pods.Status{Model: pods.ModelAirPodsPro, Left: pods.Pod{Level: 5}, Right: pods.Pod{Level: 15}, Case: pods.Pod{Level: 8}}
	out := formatHuman(ipc.FromStatus(s, true, false, time.Unix(0, 0)))
	if !strings.Contains(out, "—") {
		t.Errorf("disconnected pod should render as em dash:\n%s", out)
	}
}

func TestFormatJSON(t *testing.T) {
	out := formatJSON(snapPro(false))
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("not valid JSON: %v (%s)", err, out)
	}
	if m["model"] != "airpodspro" {
		t.Errorf("model = %v, want airpodspro", m["model"])
	}
}

func TestFormatWaybar(t *testing.T) {
	out := formatWaybar(snapPro(false))
	var w struct {
		Text    string `json:"text"`
		Tooltip string `json:"tooltip"`
		Class   string `json:"class"`
	}
	if err := json.Unmarshal([]byte(out), &w); err != nil {
		t.Fatalf("not valid JSON: %v (%s)", err, out)
	}
	if w.Class != "connected" {
		t.Errorf("class = %q, want connected", w.Class)
	}
	if !strings.Contains(w.Text, "55%") {
		t.Errorf("text = %q, want it to include a battery figure", w.Text)
	}
	if !strings.Contains(w.Tooltip, "AirPods Pro") {
		t.Errorf("tooltip = %q, want the model name", w.Tooltip)
	}
}

func TestStatusReadsFromDaemon(t *testing.T) {
	socket := tempSocket(t)
	srv, err := ipc.NewServer(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	srv.Broadcast(snapPro(false))

	var out, errb bytes.Buffer
	code := run([]string{"status", "--socket", socket, "--json"}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, errb.String())
	}
	if !strings.Contains(out.String(), "airpodspro") {
		t.Errorf("daemon read output unexpected:\n%s", out.String())
	}
}

func TestStatusReplayFallback(t *testing.T) {
	// Point --socket at a non-existent path so there is no daemon; --replay drives
	// the one-shot fallback scan.
	var out, errb bytes.Buffer
	code := run([]string{"status", "--replay", "--timeout", "3s", "--socket", tempSocket(t)}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, errb.String())
	}
	if !strings.Contains(out.String(), "AirPods Pro") || !strings.Contains(out.String(), "55%") {
		t.Errorf("replay fallback output unexpected:\n%s", out.String())
	}
}

func TestStatusWatchStreamsUntilDaemonCloses(t *testing.T) {
	socket := tempSocket(t)
	srv, err := ipc.NewServer(socket)
	if err != nil {
		t.Fatal(err)
	}
	srv.Broadcast(snapPro(false))

	out := &syncBuffer{}
	done := make(chan int, 1)
	go func() {
		done <- run([]string{"status", "--socket", socket, "--watch", "--json"}, out, io.Discard)
	}()

	waitForContains(t, out, "airpodspro")

	// A second, different broadcast also reaches the watching client.
	maxSnap := ipc.FromStatus(pods.Status{Model: pods.ModelAirPodsMax, Single: true, Left: pods.Pod{Level: 8}}, true, false, time.Unix(0, 0))
	srv.Broadcast(maxSnap)
	waitForContains(t, out, "airpodsmax")

	srv.Close() // ending the stream should make watch exit non-zero
	select {
	case code := <-done:
		if code != 1 {
			t.Errorf("watch exit = %d, want 1 after daemon close", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watch did not exit after the daemon closed")
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"bogus"}, &out, &errb)
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "unknown") {
		t.Errorf("stderr should mention unknown command: %q", errb.String())
	}
}

// syncBuffer is a concurrency-safe io.Writer for capturing streamed output.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func waitForContains(t *testing.T, b *syncBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(b.String(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("output never contained %q:\n%s", want, b.String())
}
