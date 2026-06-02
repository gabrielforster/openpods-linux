package ble

import (
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// appleCompanyID is Apple's Bluetooth company identifier (76), the key under
// which AirPods battery data appears in a Device1.ManufacturerData map.
const appleCompanyID uint16 = 0x004C

// appleManufacturerData extracts the Apple (0x004C) payload from a BlueZ
// ManufacturerData property value (a D-Bus a{qv} map). It reports false when
// the variant is the wrong shape, the Apple key is absent, or the payload is
// empty.
func appleManufacturerData(v dbus.Variant) ([]byte, bool) {
	m, ok := v.Value().(map[uint16]dbus.Variant)
	if !ok {
		return nil, false
	}
	inner, ok := m[appleCompanyID]
	if !ok {
		return nil, false
	}
	data, ok := inner.Value().([]byte)
	if !ok || len(data) == 0 {
		return nil, false
	}
	return data, true
}

// addressFromPath derives a BLE address from a BlueZ device object path such as
// "/org/bluez/hci0/dev_F4_06_8D_7A_3B_19" -> "F4:06:8D:7A:3B:19". It returns ""
// for paths that are not device objects.
func addressFromPath(path dbus.ObjectPath) string {
	s := string(path)
	i := strings.LastIndex(s, "/dev_")
	if i < 0 {
		return ""
	}
	return strings.ReplaceAll(s[i+len("/dev_"):], "_", ":")
}

// deviceState is the merged, last-known view of one BlueZ Device1 object.
type deviceState struct {
	address   string
	data      []byte // last Apple manufacturer payload
	rssi      int16
	hasRSSI   bool
	uuids     []string
	isAirPods bool
}

// deviceTracker turns BlueZ's incremental PropertiesChanged updates into Beacon
// and ConnEvent emissions. BlueZ only reports the properties that changed, so
// the tracker caches each device's latest values and decides what to emit. It
// performs no D-Bus calls and is fully unit-testable.
type deviceTracker struct {
	devices map[dbus.ObjectPath]*deviceState
}

func newDeviceTracker() *deviceTracker {
	return &deviceTracker{devices: make(map[dbus.ObjectPath]*deviceState)}
}

// update merges props (from InterfacesAdded or PropertiesChanged) for the device
// at path and returns a beacon and/or a connection event to emit, or nil for
// each when there is nothing to report. now is stamped onto any beacon.
func (t *deviceTracker) update(path dbus.ObjectPath, props map[string]dbus.Variant, now time.Time) (*Beacon, *ConnEvent) {
	st := t.devices[path]
	if st == nil {
		st = &deviceState{address: addressFromPath(path)}
		t.devices[path] = st
	}

	if v, ok := props["Address"]; ok {
		if s, ok := v.Value().(string); ok && s != "" {
			st.address = s
		}
	}
	if v, ok := props["UUIDs"]; ok {
		if u, ok := v.Value().([]string); ok {
			st.uuids = u
			st.isAirPods = IsAirPodsDevice(u)
		}
	}

	_, rssiChanged := props["RSSI"]
	if rssiChanged {
		if r, ok := props["RSSI"].Value().(int16); ok {
			st.rssi = r
			st.hasRSSI = true
		}
	}

	dataChanged := false
	if v, ok := props["ManufacturerData"]; ok {
		if d, ok := appleManufacturerData(v); ok {
			st.data = d
			dataChanged = true
		}
	}

	var beacon *Beacon
	// Emit when a battery update (or a fresh RSSI for cached data) arrives and we
	// have everything needed to apply the heuristic.
	if (dataChanged || rssiChanged) && len(st.data) > 0 && st.hasRSSI && st.address != "" {
		beacon = &Beacon{Address: st.address, Data: st.data, RSSI: st.rssi, Time: now}
	}

	var conn *ConnEvent
	if v, ok := props["Connected"]; ok {
		if c, ok := v.Value().(bool); ok && st.isAirPods {
			conn = &ConnEvent{Address: st.address, Connected: c}
		}
	}

	return beacon, conn
}
