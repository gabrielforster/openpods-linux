package pods

import "testing"

// White-box test for the nibble helper. Decode normalizes its hex string to
// uppercase, so the lowercase and invalid branches are not reachable through
// Decode; this test documents and locks the helper's full contract.
func TestNibble(t *testing.T) {
	tests := []struct {
		in   byte
		want int
	}{
		{'0', 0}, {'1', 1}, {'9', 9},
		{'A', 10}, {'B', 11}, {'F', 15},
		{'a', 10}, {'b', 11}, {'f', 15},
		{'G', 0}, {'/', 0}, {' ', 0}, {0x00, 0}, // invalid -> 0
	}
	for _, tt := range tests {
		if got := nibble(tt.in); got != tt.want {
			t.Errorf("nibble(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
