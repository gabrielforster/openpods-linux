package assets

import (
	"embed"

	"openpods-linux/pods"
)

//go:embed drawable/*.png
var drawables embed.FS

// Slot selects which figure's artwork to fetch.
type Slot int

const (
	Left Slot = iota
	Right
	Case
)

// artwork holds the base drawable names for a model, mirroring the upstream
// Android model classes (RegularPods / SinglePods subclasses).
type artwork struct {
	pod     string // left/right (and the single figure) image
	caseImg string // case image; empty for single-figure models
	single  bool
}

var modelArtwork = map[pods.Model]artwork{
	pods.ModelAirPods1:      {pod: "pod", caseImg: "pod_case"},
	pods.ModelAirPods2:      {pod: "pod", caseImg: "pod_case"},
	pods.ModelAirPods3:      {pod: "pod", caseImg: "pod_case"},
	pods.ModelAirPodsPro:    {pod: "podpro", caseImg: "podpro_case"},
	pods.ModelAirPodsPro2:   {pod: "podpro", caseImg: "podpro_case"},
	pods.ModelAirPodsPro3:   {pod: "podpro3", caseImg: "podpro_case"}, // Pro 3 keeps the Pro case art
	pods.ModelPowerbeatsPro: {pod: "powerbeatspro", caseImg: "powerbeatspro_case"},
	pods.ModelAirPodsMax:    {pod: "podmax", single: true},
	pods.ModelBeatsX:        {pod: "beatsx", single: true},
	pods.ModelBeatsFlex:     {pod: "beatsflex", single: true},
	pods.ModelBeatsSolo3:    {pod: "beatssolo3", single: true},
	pods.ModelBeatsStudio3:  {pod: "beatsstudio3", single: true},
	pods.ModelPowerbeats3:   {pod: "powerbeats3", single: true},
}

func artworkFor(m pods.Model) artwork {
	if a, ok := modelArtwork[m]; ok {
		return a
	}
	return artwork{pod: "pod", caseImg: "pod_case"} // Unknown -> plain AirPods art
}

// PodImage returns the PNG artwork for a model's slot in the given connection
// state. Single-figure models use their one image for every slot; the case slot
// of stereo models uses the case art. Disconnected slots use the dimmed variant.
func PodImage(m pods.Model, slot Slot, connected bool) []byte {
	a := artworkFor(m)
	name := a.pod
	if slot == Case && !a.single {
		name = a.caseImg
	}
	if !connected {
		name += "_disconnected"
	}
	return image(name)
}

func image(name string) []byte {
	b, err := drawables.ReadFile("drawable/" + name + ".png")
	if err != nil {
		return nil
	}
	return b
}
