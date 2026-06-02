// Package notify sends desktop notifications via the freedesktop
// org.freedesktop.Notifications service on the session bus. It fires on AirPods
// connect/disconnect. Message formatting is pure and unit-tested; the D-Bus call
// is exercised against a running notification daemon.
package notify

import (
	"fmt"

	"github.com/godbus/dbus/v5"

	"openpods-linux/ipc"
	"openpods-linux/render"
)

const (
	appName      = "OpenPods"
	appIcon      = "audio-headphones"
	expireMillis = int32(5000)
)

// Message builds the notification summary and body for a connect (connected ==
// true) or disconnect transition. The body lists battery figures; it is empty
// for a disconnect or when the snapshot has no fresh figures.
func Message(connected bool, snap ipc.Snapshot) (summary, body string) {
	if !connected {
		return render.Name(snap) + " disconnected", ""
	}
	return render.Name(snap) + " connected", render.Line(snap)
}

// Notifier sends desktop notifications.
type Notifier interface {
	Notify(summary, body string) error
	Close() error
}

type dbusNotifier struct {
	conn   *dbus.Conn
	obj    dbus.BusObject
	lastID uint32
}

// New connects to the session bus notification service.
func New() (Notifier, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("notify: connect session bus: %w", err)
	}
	obj := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	return &dbusNotifier{conn: conn, obj: obj}, nil
}

// Notify shows (or replaces, via the previous id) a desktop notification.
func (n *dbusNotifier) Notify(summary, body string) error {
	call := n.obj.Call("org.freedesktop.Notifications.Notify", 0,
		appName, n.lastID, appIcon, summary, body,
		[]string{}, map[string]dbus.Variant{}, expireMillis)
	if call.Err != nil {
		return call.Err
	}
	return call.Store(&n.lastID) // reuse id so the next notification replaces this one
}

func (n *dbusNotifier) Close() error { return n.conn.Close() }
