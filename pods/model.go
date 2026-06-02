package pods

// Model identifies an AirPods/Beats model. The zero value is ModelUnknown,
// matching the Android RegularPods fallthrough.
type Model int

const (
	ModelUnknown Model = iota
	ModelAirPods1
	ModelAirPods2
	ModelAirPods3
	ModelAirPodsPro
	ModelAirPodsPro2
	ModelAirPodsPro3
	ModelAirPodsMax
	ModelPowerbeatsPro
	ModelBeatsX
	ModelBeatsFlex
	ModelBeatsSolo3
	ModelBeatsStudio3
	ModelPowerbeats3
)

// modelNames are the canonical model strings from the Android Constants.java,
// reused as the stable identifiers for JSON/IPC and logging.
var modelNames = map[Model]string{
	ModelUnknown:       "unknown",
	ModelAirPods1:      "airpods1",
	ModelAirPods2:      "airpods2",
	ModelAirPods3:      "airpods3",
	ModelAirPodsPro:    "airpodspro",
	ModelAirPodsPro2:   "airpodspro2",
	ModelAirPodsPro3:   "airpodspro3",
	ModelAirPodsMax:    "airpodsmax",
	ModelPowerbeatsPro: "powerbeatspro",
	ModelBeatsX:        "beatsx",
	ModelBeatsFlex:     "beatsflex",
	ModelBeatsSolo3:    "beatssolo3",
	ModelBeatsStudio3:  "beatsstudio3",
	ModelPowerbeats3:   "powerbeats3",
}

// String returns the canonical model identifier (e.g. "airpodspro"). Unrecognized
// values report "unknown".
func (m Model) String() string {
	if name, ok := modelNames[m]; ok {
		return name
	}
	return "unknown"
}
