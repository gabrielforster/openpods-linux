package ble

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

// BlueZ / D-Bus names.
const (
	bluezService       = "org.bluez"
	adapterIface       = "org.bluez.Adapter1"
	deviceIface        = "org.bluez.Device1"
	objectManagerIface = "org.freedesktop.DBus.ObjectManager"
	propsIface         = "org.freedesktop.DBus.Properties"
)

// Errors surfaced by NewBlueZSource so callers can give a targeted message.
var (
	ErrNoAdapter    = errors.New("ble: no Bluetooth adapter found")
	ErrBluetoothOff = errors.New("ble: Bluetooth adapter is powered off")
)

// coarsePreFilterRSSI is the discovery-filter RSSI hint; the precise -60 gate is
// applied in code (see selector).
const coarsePreFilterRSSI int16 = -70

// bluezSource is the real Source: it drives BlueZ LE discovery over D-Bus and
// turns device advertisements into beacons and connection events via the
// deviceTracker. Following the upstream "catch, log, keep running" posture,
// recoverable D-Bus errors are logged and do not stop the scan.
type bluezSource struct {
	conn    *dbus.Conn
	adapter dbus.BusObject
	tracker *deviceTracker

	out     chan Beacon
	conns   chan ConnEvent
	signals chan *dbus.Signal

	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

// NewBlueZSource connects to the system bus, starts LE discovery on an adapter,
// and begins streaming AirPods beacons and connection events. Call Close to stop
// discovery and release the connection.
func NewBlueZSource() (Source, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("ble: connect system bus: %w", err)
	}

	adapterPath, err := findAdapter(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	adapter := conn.Object(bluezService, adapterPath)

	filter := map[string]dbus.Variant{
		"Transport":     dbus.MakeVariant("le"),
		"DuplicateData": dbus.MakeVariant(true), // keep advertisement repeats (battery changes)
		"RSSI":          dbus.MakeVariant(coarsePreFilterRSSI),
	}
	if err := adapter.Call(adapterIface+".SetDiscoveryFilter", 0, filter).Err; err != nil {
		conn.Close()
		return nil, fmt.Errorf("ble: set discovery filter: %w", err)
	}

	// Subscribe before StartDiscovery so no early advertisements are missed.
	if err := conn.AddMatchSignal(
		dbus.WithMatchSender(bluezService),
		dbus.WithMatchInterface(objectManagerIface),
		dbus.WithMatchMember("InterfacesAdded"),
	); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ble: subscribe InterfacesAdded: %w", err)
	}
	if err := conn.AddMatchSignal(
		dbus.WithMatchSender(bluezService),
		dbus.WithMatchInterface(propsIface),
		dbus.WithMatchMember("PropertiesChanged"),
	); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ble: subscribe PropertiesChanged: %w", err)
	}

	if err := adapter.Call(adapterIface+".StartDiscovery", 0).Err; err != nil {
		conn.Close()
		return nil, fmt.Errorf("ble: start discovery: %w", err)
	}

	s := &bluezSource{
		conn:    conn,
		adapter: adapter,
		tracker: newDeviceTracker(),
		out:     make(chan Beacon),
		conns:   make(chan ConnEvent),
		signals: make(chan *dbus.Signal, 64),
		done:    make(chan struct{}),
	}
	conn.Signal(s.signals)

	s.wg.Add(1)
	go s.run()
	return s, nil
}

func (s *bluezSource) Beacons() <-chan Beacon        { return s.out }
func (s *bluezSource) Connections() <-chan ConnEvent { return s.conns }

func (s *bluezSource) Close() error {
	s.closeOnce.Do(func() { close(s.done) })
	if s.adapter != nil {
		_ = s.adapter.Call(adapterIface+".StopDiscovery", 0).Err // best effort
	}
	s.conn.RemoveSignal(s.signals)
	err := s.conn.Close()
	s.wg.Wait()
	return err
}

func (s *bluezSource) run() {
	defer s.wg.Done()
	defer close(s.out)
	defer close(s.conns)

	// Seed from devices BlueZ already knows about.
	if objects, err := managedObjects(s.conn); err != nil {
		slog.Warn("openpods: initial device enumeration failed", "err", err)
	} else {
		for path, ifaces := range objects {
			if props, ok := ifaces[deviceIface]; ok {
				if !s.feed(path, props) {
					return
				}
			}
		}
	}

	for {
		select {
		case <-s.done:
			return
		case sig, ok := <-s.signals:
			if !ok {
				return
			}
			if !s.handleSignal(sig) {
				return
			}
		}
	}
}

func (s *bluezSource) handleSignal(sig *dbus.Signal) bool {
	switch sig.Name {
	case objectManagerIface + ".InterfacesAdded":
		path, ifaces, ok := parseInterfacesAdded(sig.Body)
		if !ok {
			return true
		}
		if props, ok := ifaces[deviceIface]; ok {
			return s.feed(path, props)
		}
	case propsIface + ".PropertiesChanged":
		iface, changed, ok := parsePropertiesChanged(sig.Body)
		if !ok || iface != deviceIface {
			return true
		}
		return s.feed(sig.Path, changed)
	}
	return true
}

func (s *bluezSource) feed(path dbus.ObjectPath, props map[string]dbus.Variant) bool {
	beacon, conn := s.tracker.update(path, props, time.Now())
	if beacon != nil && !sendOrDone(s.out, *beacon, s.done) {
		return false
	}
	if conn != nil && !sendOrDone(s.conns, *conn, s.done) {
		return false
	}
	return true
}

func sendOrDone[T any](ch chan T, v T, done <-chan struct{}) bool {
	select {
	case ch <- v:
		return true
	case <-done:
		return false
	}
}

func findAdapter(conn *dbus.Conn) (dbus.ObjectPath, error) {
	objects, err := managedObjects(conn)
	if err != nil {
		return "", err
	}
	return selectAdapter(objects)
}

func managedObjects(conn *dbus.Conn) (map[dbus.ObjectPath]map[string]map[string]dbus.Variant, error) {
	var objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	obj := conn.Object(bluezService, "/")
	if err := obj.Call(objectManagerIface+".GetManagedObjects", 0).Store(&objects); err != nil {
		return nil, fmt.Errorf("ble: get managed objects: %w", err)
	}
	return objects, nil
}

// --- testable helpers ---

// selectAdapter picks an adapter from a GetManagedObjects result, preferring a
// powered one. It returns ErrBluetoothOff when adapters exist but none are
// powered, and ErrNoAdapter when there are none.
func selectAdapter(objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant) (dbus.ObjectPath, error) {
	hasAdapter := false
	for path, ifaces := range objects {
		props, ok := ifaces[adapterIface]
		if !ok {
			continue
		}
		hasAdapter = true
		if powered, ok := props["Powered"].Value().(bool); ok && powered {
			return path, nil
		}
	}
	if !hasAdapter {
		return "", ErrNoAdapter
	}
	return "", ErrBluetoothOff
}

// parsePropertiesChanged unpacks an org.freedesktop.DBus.Properties.PropertiesChanged
// signal body: [interface_name, changed_properties, invalidated_properties].
func parsePropertiesChanged(body []any) (string, map[string]dbus.Variant, bool) {
	if len(body) < 2 {
		return "", nil, false
	}
	iface, ok := body[0].(string)
	if !ok {
		return "", nil, false
	}
	changed, ok := body[1].(map[string]dbus.Variant)
	if !ok {
		return "", nil, false
	}
	return iface, changed, true
}

// parseInterfacesAdded unpacks an org.freedesktop.DBus.ObjectManager.InterfacesAdded
// signal body: [object_path, interfaces_and_properties].
func parseInterfacesAdded(body []any) (dbus.ObjectPath, map[string]map[string]dbus.Variant, bool) {
	if len(body) < 2 {
		return "", nil, false
	}
	path, ok := body[0].(dbus.ObjectPath)
	if !ok {
		return "", nil, false
	}
	ifaces, ok := body[1].(map[string]map[string]dbus.Variant)
	if !ok {
		return "", nil, false
	}
	return path, ifaces, true
}
