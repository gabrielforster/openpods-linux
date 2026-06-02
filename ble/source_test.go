package ble_test

import (
	"testing"

	"openpods-linux/ble"
)

func TestIsAirPodsDevice(t *testing.T) {
	tests := []struct {
		name  string
		uuids []string
		want  bool
	}{
		{"nil", nil, false},
		{"empty", []string{}, false},
		{"published UUID", []string{"74ec2172-0bad-4d01-8f77-997b2be0722a"}, true},
		{"reversed UUID", []string{"2a72e02b-7b99-778f-014d-ad0b7221ec74"}, true},
		{"uppercase match", []string{"74EC2172-0BAD-4D01-8F77-997B2BE0722A"}, true},
		{"among other services", []string{"0000110b-0000-1000-8000-00805f9b34fb", "74ec2172-0bad-4d01-8f77-997b2be0722a"}, true},
		{"no match", []string{"0000110b-0000-1000-8000-00805f9b34fb"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ble.IsAirPodsDevice(tt.uuids); got != tt.want {
				t.Errorf("IsAirPodsDevice(%v) = %v, want %v", tt.uuids, got, tt.want)
			}
		})
	}
}
