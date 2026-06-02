package ble

import (
	"errors"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestSelectAdapter(t *testing.T) {
	adapter := func(powered bool) map[string]map[string]dbus.Variant {
		return map[string]map[string]dbus.Variant{
			adapterIface: {"Powered": dbus.MakeVariant(powered)},
		}
	}
	device := map[string]map[string]dbus.Variant{
		deviceIface: {"Address": dbus.MakeVariant("AA:BB")},
	}

	t.Run("single powered adapter", func(t *testing.T) {
		objs := map[dbus.ObjectPath]map[string]map[string]dbus.Variant{
			"/org/bluez/hci0": adapter(true),
		}
		got, err := selectAdapter(objs)
		if err != nil || got != "/org/bluez/hci0" {
			t.Errorf("selectAdapter = (%q,%v), want (/org/bluez/hci0, nil)", got, err)
		}
	})

	t.Run("prefers the powered adapter", func(t *testing.T) {
		objs := map[dbus.ObjectPath]map[string]map[string]dbus.Variant{
			"/org/bluez/hci0": adapter(false),
			"/org/bluez/hci1": adapter(true),
		}
		got, err := selectAdapter(objs)
		if err != nil || got != "/org/bluez/hci1" {
			t.Errorf("selectAdapter = (%q,%v), want (/org/bluez/hci1, nil)", got, err)
		}
	})

	t.Run("adapter present but powered off", func(t *testing.T) {
		objs := map[dbus.ObjectPath]map[string]map[string]dbus.Variant{
			"/org/bluez/hci0": adapter(false),
		}
		if _, err := selectAdapter(objs); !errors.Is(err, ErrBluetoothOff) {
			t.Errorf("err = %v, want ErrBluetoothOff", err)
		}
	})

	t.Run("no adapter at all", func(t *testing.T) {
		objs := map[dbus.ObjectPath]map[string]map[string]dbus.Variant{
			"/org/bluez/hci0/dev_AA_BB": device,
		}
		if _, err := selectAdapter(objs); !errors.Is(err, ErrNoAdapter) {
			t.Errorf("err = %v, want ErrNoAdapter", err)
		}
	})
}

func TestParsePropertiesChanged(t *testing.T) {
	changed := map[string]dbus.Variant{"RSSI": dbus.MakeVariant(int16(-50))}

	t.Run("valid", func(t *testing.T) {
		iface, got, ok := parsePropertiesChanged([]any{"org.bluez.Device1", changed, []string{}})
		if !ok || iface != "org.bluez.Device1" || len(got) != 1 {
			t.Errorf("parse = (%q,%v,%v)", iface, got, ok)
		}
	})
	t.Run("too short", func(t *testing.T) {
		if _, _, ok := parsePropertiesChanged([]any{"only-one"}); ok {
			t.Error("expected ok=false for short body")
		}
	})
	t.Run("wrong types", func(t *testing.T) {
		if _, _, ok := parsePropertiesChanged([]any{42, "nope"}); ok {
			t.Error("expected ok=false for wrong types")
		}
	})
}

func TestParseInterfacesAdded(t *testing.T) {
	ifaces := map[string]map[string]dbus.Variant{
		deviceIface: {"Address": dbus.MakeVariant("AA:BB")},
	}

	t.Run("valid", func(t *testing.T) {
		path, got, ok := parseInterfacesAdded([]any{dbus.ObjectPath("/org/bluez/hci0/dev_AA_BB"), ifaces})
		if !ok || path != "/org/bluez/hci0/dev_AA_BB" || len(got) != 1 {
			t.Errorf("parse = (%q,%v,%v)", path, got, ok)
		}
	})
	t.Run("too short", func(t *testing.T) {
		if _, _, ok := parseInterfacesAdded([]any{dbus.ObjectPath("/x")}); ok {
			t.Error("expected ok=false for short body")
		}
	})
	t.Run("wrong types", func(t *testing.T) {
		if _, _, ok := parseInterfacesAdded([]any{"not-a-path", 42}); ok {
			t.Error("expected ok=false for wrong types")
		}
	})
}
