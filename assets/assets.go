// Package assets embeds artwork ported from the upstream Android app (GPLv3,
// © Federico Dossena) for the tray and GUI frontends.
package assets

import _ "embed"

//go:embed icon.png
var iconPNG []byte

// Icon returns the OpenPods application icon as PNG bytes (512×512 RGBA).
func Icon() []byte { return iconPNG }
