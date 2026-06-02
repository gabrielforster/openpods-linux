package ble

import (
	"bytes"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

var trackTime = time.Unix(1_700_000_000, 0)

func applePayload() []byte {
	// 27-byte AirPods Pro beacon (same vector used elsewhere).
	p := make([]byte, 27)
	hexBytes := []byte{0x07, 0x19, 0x01, 0x0E, 0x20, 0x20, 0xA5, 0x08}
	copy(p, hexBytes)
	return p
}

func manufacturerVariant(companyID uint16, data []byte) dbus.Variant {
	return dbus.MakeVariant(map[uint16]dbus.Variant{companyID: dbus.MakeVariant(data)})
}

func TestAppleManufacturerData(t *testing.T) {
	payload := applePayload()
	t.Run("apple data present", func(t *testing.T) {
		got, ok := appleManufacturerData(manufacturerVariant(0x004C, payload))
		if !ok || !bytes.Equal(got, payload) {
			t.Errorf("got (%v,%v), want the payload, true", got, ok)
		}
	})
	t.Run("non-apple company id", func(t *testing.T) {
		if _, ok := appleManufacturerData(manufacturerVariant(0x0006, payload)); ok {
			t.Error("expected ok=false for non-Apple company id")
		}
	})
	t.Run("wrong outer type", func(t *testing.T) {
		if _, ok := appleManufacturerData(dbus.MakeVariant("nope")); ok {
			t.Error("expected ok=false for non-map variant")
		}
	})
	t.Run("empty payload", func(t *testing.T) {
		if _, ok := appleManufacturerData(manufacturerVariant(0x004C, nil)); ok {
			t.Error("expected ok=false for empty payload")
		}
	})
}

func TestAddressFromPath(t *testing.T) {
	tests := []struct {
		path dbus.ObjectPath
		want string
	}{
		{"/org/bluez/hci0/dev_F4_06_8D_7A_3B_19", "F4:06:8D:7A:3B:19"},
		{"/org/bluez/hci0", ""},
		{"/org/bluez/hci0/dev_AA_BB", "AA:BB"},
	}
	for _, tt := range tests {
		if got := addressFromPath(tt.path); got != tt.want {
			t.Errorf("addressFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestDeviceTrackerFullProps(t *testing.T) {
	tr := newDeviceTracker()
	beacon, conn := tr.update("/org/bluez/hci0/dev_F4_06_8D_7A_3B_19", map[string]dbus.Variant{
		"Address":          dbus.MakeVariant("F4:06:8D:7A:3B:19"),
		"RSSI":             dbus.MakeVariant(int16(-45)),
		"Connected":        dbus.MakeVariant(true),
		"UUIDs":            dbus.MakeVariant([]string{"74ec2172-0bad-4d01-8f77-997b2be0722a"}),
		"ManufacturerData": manufacturerVariant(0x004C, applePayload()),
	}, trackTime)

	if beacon == nil {
		t.Fatal("expected a beacon")
	}
	if beacon.Address != "F4:06:8D:7A:3B:19" || beacon.RSSI != -45 || !beacon.Time.Equal(trackTime) {
		t.Errorf("beacon = %+v", beacon)
	}
	if conn == nil || !conn.Connected {
		t.Errorf("expected a connected ConnEvent, got %+v", conn)
	}
}

func TestDeviceTrackerIncrementalDataThenRSSI(t *testing.T) {
	tr := newDeviceTracker()
	path := dbus.ObjectPath("/org/bluez/hci0/dev_F4_06_8D_7A_3B_19")

	// ManufacturerData arrives but no RSSI known yet -> cannot gate -> no beacon.
	if b, _ := tr.update(path, map[string]dbus.Variant{
		"ManufacturerData": manufacturerVariant(0x004C, applePayload()),
	}, trackTime); b != nil {
		t.Fatalf("did not expect a beacon before RSSI is known, got %+v", b)
	}

	// RSSI arrives -> now we can emit, using the cached data.
	b, _ := tr.update(path, map[string]dbus.Variant{
		"RSSI": dbus.MakeVariant(int16(-50)),
	}, trackTime)
	if b == nil {
		t.Fatal("expected a beacon once RSSI is known")
	}
	if b.RSSI != -50 || !bytes.Equal(b.Data, applePayload()) {
		t.Errorf("beacon = %+v", b)
	}
	// Address was derived from the object path (never supplied in props).
	if b.Address != "F4:06:8D:7A:3B:19" {
		t.Errorf("Address = %q, want derived from path", b.Address)
	}
}

func TestDeviceTrackerIgnoresNonApple(t *testing.T) {
	tr := newDeviceTracker()
	b, _ := tr.update("/org/bluez/hci0/dev_11_22_33_44_55_66", map[string]dbus.Variant{
		"RSSI":             dbus.MakeVariant(int16(-40)),
		"ManufacturerData": manufacturerVariant(0x0006, []byte{1, 2, 3}), // Microsoft, not Apple
	}, trackTime)
	if b != nil {
		t.Errorf("non-Apple advertisement should not emit a beacon, got %+v", b)
	}
}

func TestDeviceTrackerIgnoresNonAirPodsConnection(t *testing.T) {
	tr := newDeviceTracker()
	_, conn := tr.update("/org/bluez/hci0/dev_11_22_33_44_55_66", map[string]dbus.Variant{
		"Connected": dbus.MakeVariant(true),
		"UUIDs":     dbus.MakeVariant([]string{"0000110b-0000-1000-8000-00805f9b34fb"}),
	}, trackTime)
	if conn != nil {
		t.Errorf("non-AirPods connection should not emit a ConnEvent, got %+v", conn)
	}
}
