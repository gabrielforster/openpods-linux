package assets_test

import (
	"bytes"
	"testing"

	"openpods-linux/assets"
	"openpods-linux/pods"
)

func isPNG(b []byte) bool {
	return bytes.HasPrefix(b, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})
}

func TestPodImageEveryModel(t *testing.T) {
	models := []pods.Model{
		pods.ModelAirPods1, pods.ModelAirPods2, pods.ModelAirPods3,
		pods.ModelAirPodsPro, pods.ModelAirPodsPro2, pods.ModelAirPodsPro3,
		pods.ModelAirPodsMax, pods.ModelPowerbeatsPro, pods.ModelBeatsX,
		pods.ModelBeatsFlex, pods.ModelBeatsSolo3, pods.ModelBeatsStudio3,
		pods.ModelPowerbeats3, pods.ModelUnknown,
	}
	for _, m := range models {
		for _, slot := range []assets.Slot{assets.Left, assets.Right, assets.Case} {
			for _, connected := range []bool{true, false} {
				b := assets.PodImage(m, slot, connected)
				if !isPNG(b) {
					t.Errorf("PodImage(%s, %v, connected=%v) is not a PNG (%d bytes)", m, slot, connected, len(b))
				}
			}
		}
	}
}

func TestPodImageDisconnectedDiffers(t *testing.T) {
	on := assets.PodImage(pods.ModelAirPodsPro, assets.Left, true)
	off := assets.PodImage(pods.ModelAirPodsPro, assets.Left, false)
	if bytes.Equal(on, off) {
		t.Error("connected and disconnected images should differ")
	}
}

func TestPodImageProUsesProArtwork(t *testing.T) {
	// AirPods Pro left pod uses podpro; AirPods (gen1) uses the plain pod.
	pro := assets.PodImage(pods.ModelAirPodsPro, assets.Left, true)
	gen1 := assets.PodImage(pods.ModelAirPods1, assets.Left, true)
	if bytes.Equal(pro, gen1) {
		t.Error("AirPods Pro and AirPods 1 should use different pod artwork")
	}
}
