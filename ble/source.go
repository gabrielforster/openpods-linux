package ble

import "strings"

// Source delivers raw BLE observations. The BlueZ backend (bluez.go) implements
// it over D-Bus; tests and the --replay mode provide in-memory fakes. The two
// channels are independent: Beacons carries decoded-pending advertisements,
// Connections carries AirPods audio-link state changes.
type Source interface {
	Beacons() <-chan Beacon
	Connections() <-chan ConnEvent
	Close() error
}

// ConnEvent reports an AirPods audio device connecting or disconnecting. A
// Source only emits these for devices that match the AirPods service UUIDs.
type ConnEvent struct {
	Address   string
	Connected bool
}

// AirPodsServiceUUIDs are the service UUIDs the Android app uses to confirm a
// connected device is AirPods (PodsService.checkUUID): the published UUID and
// its byte-reversed form.
var AirPodsServiceUUIDs = []string{
	"74ec2172-0bad-4d01-8f77-997b2be0722a",
	"2a72e02b-7b99-778f-014d-ad0b7221ec74",
}

// IsAirPodsDevice reports whether any of the device's service UUIDs match the
// AirPods service UUIDs. Comparison is case-insensitive (BlueZ reports UUIDs
// lowercase; the Android constants are lowercase too).
func IsAirPodsDevice(uuids []string) bool {
	for _, u := range uuids {
		for _, known := range AirPodsServiceUUIDs {
			if strings.EqualFold(u, known) {
				return true
			}
		}
	}
	return false
}
