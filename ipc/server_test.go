package ipc_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"openpods-linux/ipc"
	"openpods-linux/pods"
)

func fixture(model pods.Model, at time.Time) ipc.Snapshot {
	s := pods.Status{Model: model, Left: pods.Pod{Level: 5}, Right: pods.Pod{Level: 10}, Case: pods.Pod{Level: 8}}
	return ipc.FromStatus(s, true, false, at)
}

func tempSocket(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "openpods.sock")
}

func TestServerSendsCurrentOnConnect(t *testing.T) {
	path := tempSocket(t)
	srv, err := ipc.NewServer(path)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	srv.Broadcast(fixture(pods.ModelAirPodsPro, snapTime))

	cl, err := ipc.Dial(path)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	snap, err := cl.Read()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Model != "airpodspro" {
		t.Errorf("Model = %q, want airpodspro", snap.Model)
	}
}

func TestServerStreamsBroadcasts(t *testing.T) {
	path := tempSocket(t)
	srv, err := ipc.NewServer(path)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	srv.Broadcast(fixture(pods.ModelAirPodsPro, snapTime))
	cl, err := ipc.Dial(path)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	first, err := cl.Read()
	if err != nil || first.Model != "airpodspro" {
		t.Fatalf("first read = %+v, %v", first, err)
	}

	// A later broadcast reaches the connected client.
	srv.Broadcast(fixture(pods.ModelAirPodsMax, snapTime.Add(time.Minute)))
	second, err := cl.Read()
	if err != nil {
		t.Fatal(err)
	}
	if second.Model != "airpodsmax" {
		t.Errorf("second Model = %q, want airpodsmax", second.Model)
	}
}

func TestServerSupportsMultipleClients(t *testing.T) {
	path := tempSocket(t)
	srv, err := ipc.NewServer(path)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	srv.Broadcast(fixture(pods.ModelAirPodsPro, snapTime))

	for i := range 3 {
		cl, err := ipc.Dial(path)
		if err != nil {
			t.Fatalf("client %d dial: %v", i, err)
		}
		snap, err := cl.Read()
		if err != nil || snap.Model != "airpodspro" {
			t.Errorf("client %d read = %+v, %v", i, snap, err)
		}
		cl.Close()
	}
}

func TestClientReadErrorsAfterServerClose(t *testing.T) {
	path := tempSocket(t)
	srv, err := ipc.NewServer(path)
	if err != nil {
		t.Fatal(err)
	}
	cl, err := ipc.Dial(path)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	srv.Close()
	if _, err := cl.Read(); err == nil {
		t.Error("Read should error after the server closes")
	}
}

func TestDefaultSocketPath(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	if got := ipc.DefaultSocketPath(); got != "/run/user/1000/openpods.sock" {
		t.Errorf("DefaultSocketPath() = %q, want /run/user/1000/openpods.sock", got)
	}
	t.Setenv("XDG_RUNTIME_DIR", "")
	if got := ipc.DefaultSocketPath(); !strings.HasSuffix(got, "openpods.sock") {
		t.Errorf("fallback DefaultSocketPath() = %q, want suffix openpods.sock", got)
	}
}
