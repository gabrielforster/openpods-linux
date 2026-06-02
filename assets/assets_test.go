package assets_test

import (
	"bytes"
	"testing"

	"openpods-linux/assets"
)

func TestIconIsPNG(t *testing.T) {
	b := assets.Icon()
	if len(b) == 0 {
		t.Fatal("Icon() returned no bytes")
	}
	if !bytes.HasPrefix(b, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}) {
		t.Error("Icon() is not a PNG")
	}
}
