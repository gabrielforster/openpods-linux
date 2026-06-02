package pods_test

import (
	"encoding/hex"
	"errors"
	"testing"

	"openpods-linux/pods"
)

// body builds a 27-byte beacon payload from the 6 "meaningful" bytes (bytes
// 2..7) given as a 12-char hex string. Bytes 0..1 are the fixed prefix
// 0x07 0x19 and bytes 8..26 are zero padding. This keeps each test vector as
// raw bytes that are completely independent of the decoder's own logic.
//
// Hex-char layout inside the resulting 54-char uppercase string (see
// beacon-protocol.md §3): char 6-9 = model id, 10 = flip, 11 = in-ear,
// 12 = battery A, 13 = battery B / single, 14 = charge, 15 = case.
func body(t *testing.T, b2to7 string) []byte {
	t.Helper()
	if len(b2to7) != 12 {
		t.Fatalf("body: want 12 hex chars (bytes 2..7), got %d", len(b2to7))
	}
	full := "0719" + b2to7 + // bytes 0,1 prefix + bytes 2..7
		"0000000000000000000000000000000000000000000000000000000000000000000000000000" // bytes 8..26 (19 bytes)
	full = full[:54]
	p, err := hex.DecodeString(full)
	if err != nil {
		t.Fatalf("body: bad hex %q: %v", full, err)
	}
	if len(p) != 27 {
		t.Fatalf("body: want 27 bytes, got %d", len(p))
	}
	return p
}

func disc() pods.Pod { return pods.Pod{Level: 15} }

func assertStatus(t *testing.T, got pods.Status, want pods.Status) {
	t.Helper()
	if got.Model != want.Model {
		t.Errorf("Model = %v (%s), want %v (%s)", got.Model, got.Model, want.Model, want.Model)
	}
	if got.Single != want.Single {
		t.Errorf("Single = %v, want %v", got.Single, want.Single)
	}
	if got.Flipped != want.Flipped {
		t.Errorf("Flipped = %v, want %v", got.Flipped, want.Flipped)
	}
	if got.Left != want.Left {
		t.Errorf("Left = %+v, want %+v", got.Left, want.Left)
	}
	if got.Right != want.Right {
		t.Errorf("Right = %+v, want %+v", got.Right, want.Right)
	}
	if got.Case != want.Case {
		t.Errorf("Case = %+v, want %+v", got.Case, want.Case)
	}
}

// TestDecodeModels covers every model id from beacon-protocol.md §6, including
// the single/stereo classification and the "unknown" fallthrough.
//
// All rows share the body suffix "20A508": flip nibble 2 (not flipped),
// in-ear 0, battery A (char 12) = 0xA = 10, battery B (char 13) = 0x5 = 5,
// charge 0, case (char 15) = 8. Per the Android code (PodsStatus.java:50-51),
// when NOT flipped left = char 13 and right = char 12, so a stereo decode
// yields Left=5, Right=10, Case=8.
func TestDecodeModels(t *testing.T) {
	stereo := func(m pods.Model) pods.Status {
		return pods.Status{
			Model: m,
			Left:  pods.Pod{Level: 5},
			Right: pods.Pod{Level: 10},
			Case:  pods.Pod{Level: 8},
		}
	}
	single := func(m pods.Model) pods.Status {
		// single figure = char 13 = 5; right/case disconnected.
		return pods.Status{
			Model:  m,
			Single: true,
			Left:   pods.Pod{Level: 5},
			Right:  disc(),
			Case:   disc(),
		}
	}

	tests := []struct {
		name   string
		idFull string // 4 hex chars at positions 6-9
		want   pods.Status
	}{
		{"AirPods 1", "0220", stereo(pods.ModelAirPods1)},
		{"AirPods 2", "0F20", stereo(pods.ModelAirPods2)},
		{"AirPods 3", "1320", stereo(pods.ModelAirPods3)},
		{"AirPods Pro", "0E20", stereo(pods.ModelAirPodsPro)},
		{"AirPods Pro 2 (1420)", "1420", stereo(pods.ModelAirPodsPro2)},
		{"AirPods Pro 2 (2420)", "2420", stereo(pods.ModelAirPodsPro2)},
		{"AirPods Pro 3", "2720", stereo(pods.ModelAirPodsPro3)},
		{"AirPods Max", "0A20", single(pods.ModelAirPodsMax)},       // idSingle = 'A'
		{"Powerbeats Pro", "0B20", stereo(pods.ModelPowerbeatsPro)}, // idSingle = 'B'
		{"Beats X", "0520", single(pods.ModelBeatsX)},
		{"Beats Flex", "1020", single(pods.ModelBeatsFlex)},
		{"Beats Solo 3", "0620", single(pods.ModelBeatsSolo3)},
		{"Beats Studio 3", "0920", single(pods.ModelBeatsStudio3)}, // idSingle = '9'
		{"Powerbeats 3", "0320", single(pods.ModelPowerbeats3)},
		{"Unknown", "FFFF", stereo(pods.ModelUnknown)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := body(t, "01"+tt.idFull+"20A508")
			got, err := pods.Decode(p)
			if err != nil {
				t.Fatalf("Decode error: %v", err)
			}
			assertStatus(t, got, tt.want)
		})
	}
}

// TestDecodeFlipped verifies that when the flip flag is set, the left/right
// battery nibbles, the charge bits (0<->1), and the in-ear bits (1<->3) all
// swap. Body "0E20" + "08" + "39" + "17":
//
//	char 10 = 0 -> flipped (bit 1 clear)
//	char 11 = 8 -> in-ear nibble bit 3 set
//	char 12 = 3 (battery A), char 13 = 9 (battery B)
//	char 14 = 1 -> charge nibble bit 0 set, char 15 = 7 (case)
func TestDecodeFlipped(t *testing.T) {
	p := body(t, "010E20083917")
	got, err := pods.Decode(p)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	want := pods.Status{
		Model:   pods.ModelAirPodsPro,
		Flipped: true,
		// flipped: left = char 12 = 3, right = char 13 = 9
		Left:  pods.Pod{Level: 3, Charging: false, InEar: true}, // in-ear bit 3
		Right: pods.Pod{Level: 9, Charging: true, InEar: false}, // charge bit 0
		Case:  pods.Pod{Level: 7},
	}
	assertStatus(t, got, want)
}

// TestDecodeCharging covers charge-bit combinations (not flipped).
// Body battery bytes fixed at char12=0xA(10), char13=0x8(8) -> left=8, right=10.
func TestDecodeCharging(t *testing.T) {
	tests := []struct {
		name                string
		chargeNibble        string // char 14
		wantL, wantR, wantC bool
	}{
		{"all charging", "7", true, true, true}, // bits 0,1,2
		{"left only", "1", true, false, false},  // bit 0
		{"right only", "2", false, true, false}, // bit 1
		{"case only", "4", false, false, true},  // bit 2
		{"none", "0", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// byte 7 = charge<<4 | case; case nibble = 5.
			p := body(t, "010E2020A8"+tt.chargeNibble+"5")
			got, err := pods.Decode(p)
			if err != nil {
				t.Fatalf("Decode error: %v", err)
			}
			if got.Left.Charging != tt.wantL {
				t.Errorf("Left.Charging = %v, want %v", got.Left.Charging, tt.wantL)
			}
			if got.Right.Charging != tt.wantR {
				t.Errorf("Right.Charging = %v, want %v", got.Right.Charging, tt.wantR)
			}
			if got.Case.Charging != tt.wantC {
				t.Errorf("Case.Charging = %v, want %v", got.Case.Charging, tt.wantC)
			}
		})
	}
}

// TestDecodeInEar covers in-ear bit decoding (not flipped): bit 1 = left,
// bit 3 = right.
func TestDecodeInEar(t *testing.T) {
	tests := []struct {
		name         string
		inEarNibble  string // char 11
		wantL, wantR bool
	}{
		{"neither", "0", false, false},
		{"left only", "2", true, false},  // bit 1
		{"right only", "8", false, true}, // bit 3
		{"both", "A", true, true},        // bits 1 and 3
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// byte 5 = flip<<4 | inEar; flip nibble = 2 (not flipped).
			p := body(t, "010E202"+tt.inEarNibble+"A508")
			got, err := pods.Decode(p)
			if err != nil {
				t.Fatalf("Decode error: %v", err)
			}
			if got.Left.InEar != tt.wantL {
				t.Errorf("Left.InEar = %v, want %v", got.Left.InEar, tt.wantL)
			}
			if got.Right.InEar != tt.wantR {
				t.Errorf("Right.InEar = %v, want %v", got.Right.InEar, tt.wantR)
			}
		})
	}
}

// TestDecodeSingle verifies the single-device decode path: the figure is read
// from char 13 and its charge bit is bit 0 of the charge nibble, regardless of
// the flip flag.
func TestDecodeSingle(t *testing.T) {
	t.Run("not flipped", func(t *testing.T) {
		// char12=7, char13=4 (single=4); charge nibble=1 (bit0 -> single charging); case=9.
		p := body(t, "010A20207419")
		got, err := pods.Decode(p)
		if err != nil {
			t.Fatalf("Decode error: %v", err)
		}
		want := pods.Status{
			Model:  pods.ModelAirPodsMax,
			Single: true,
			Left:   pods.Pod{Level: 4, Charging: true},
			Right:  disc(),
			Case:   disc(),
		}
		assertStatus(t, got, want)
	})

	t.Run("flipped does not move the single figure", func(t *testing.T) {
		// flip nibble=0 (flipped), char12=7, char13=4; single must still read char13=4.
		p := body(t, "010A20087419")
		got, err := pods.Decode(p)
		if err != nil {
			t.Fatalf("Decode error: %v", err)
		}
		want := pods.Status{
			Model:   pods.ModelAirPodsMax,
			Single:  true,
			Flipped: true,
			Left:    pods.Pod{Level: 4, Charging: true}, // char13, charge bit0 (not swapped)
			Right:   disc(),
			Case:    disc(),
		}
		assertStatus(t, got, want)
	})
}

// TestDecodeDisconnectedNibble covers the 0xF (15) "disconnected" nibble.
func TestDecodeDisconnectedNibble(t *testing.T) {
	// char12=F, char13=F, charge=0, case=F.
	p := body(t, "010E2020FF0F")
	got, err := pods.Decode(p)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	for name, pod := range map[string]pods.Pod{"Left": got.Left, "Right": got.Right, "Case": got.Case} {
		if pod.Level != 15 {
			t.Errorf("%s.Level = %d, want 15", name, pod.Level)
		}
		if pod.Connected() {
			t.Errorf("%s.Connected() = true, want false", name)
		}
		if !pod.Disconnected() {
			t.Errorf("%s.Disconnected() = false, want true", name)
		}
		if _, ok := pod.Percent(); ok {
			t.Errorf("%s.Percent() ok = true, want false", name)
		}
		if pod.Low() {
			t.Errorf("%s.Low() = true, want false (disconnected is not low)", name)
		}
	}
}

// TestDecodeOutOfRangeNibble covers values 11-14 which are neither a battery
// level nor the disconnected sentinel: treated as not-connected/unknown.
func TestDecodeOutOfRangeNibble(t *testing.T) {
	// char12=C(12), char13=B(11), charge=0, case=D(13).
	p := body(t, "010E2020CB0D")
	got, err := pods.Decode(p)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	// not flipped: left=char13=11, right=char12=12, case=char15=13.
	checks := []struct {
		name  string
		pod   pods.Pod
		level int
	}{
		{"Left", got.Left, 11},
		{"Right", got.Right, 12},
		{"Case", got.Case, 13},
	}
	for _, c := range checks {
		if c.pod.Level != c.level {
			t.Errorf("%s.Level = %d, want %d", c.name, c.pod.Level, c.level)
		}
		if c.pod.Connected() {
			t.Errorf("%s.Connected() = true, want false", c.name)
		}
		if c.pod.Disconnected() {
			t.Errorf("%s.Disconnected() = true, want false (11-14 is not the 15 sentinel)", c.name)
		}
		if _, ok := c.pod.Percent(); ok {
			t.Errorf("%s.Percent() ok = true, want false", c.name)
		}
	}
}

// TestDecodeMalformed verifies that invalid payloads are rejected with
// ErrInvalidPayload (length != 27, or wrong 0x07 0x19 prefix).
func TestDecodeMalformed(t *testing.T) {
	valid := body(t, "010E2020A508") // 27 bytes, good prefix

	short := make([]byte, 26)
	copy(short, valid)

	long := make([]byte, 28)
	copy(long, valid)

	badByte0 := append([]byte(nil), valid...)
	badByte0[0] = 0x08

	badByte1 := append([]byte(nil), valid...)
	badByte1[1] = 0x18

	tests := []struct {
		name    string
		payload []byte
	}{
		{"nil", nil},
		{"empty", []byte{}},
		{"too short (26)", short},
		{"too long (28)", long},
		{"wrong byte0", badByte0},
		{"wrong byte1", badByte1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pods.Decode(tt.payload)
			if err == nil {
				t.Fatalf("Decode(%v) = nil error, want error", tt.payload)
			}
			if !errors.Is(err, pods.ErrInvalidPayload) {
				t.Errorf("Decode error = %v, want errors.Is ErrInvalidPayload", err)
			}
		})
	}

	// Sanity: the otherwise-valid base payload must decode cleanly.
	if _, err := pods.Decode(valid); err != nil {
		t.Fatalf("valid payload failed to decode: %v", err)
	}
}

func TestPodMethods(t *testing.T) {
	tests := []struct {
		level        int
		wantPercent  int
		wantOK       bool
		connected    bool
		low          bool
		disconnected bool
	}{
		{0, 5, true, true, true, false},
		{1, 15, true, true, true, false},
		{2, 25, true, true, false, false},
		{5, 55, true, true, false, false},
		{9, 95, true, true, false, false},
		{10, 100, true, true, false, false},
		{11, 0, false, false, false, false},
		{14, 0, false, false, false, false},
		{15, 0, false, false, false, true},
	}
	for _, tt := range tests {
		p := pods.Pod{Level: tt.level}
		gotPct, gotOK := p.Percent()
		if gotPct != tt.wantPercent || gotOK != tt.wantOK {
			t.Errorf("level %d: Percent() = (%d,%v), want (%d,%v)", tt.level, gotPct, gotOK, tt.wantPercent, tt.wantOK)
		}
		if p.Connected() != tt.connected {
			t.Errorf("level %d: Connected() = %v, want %v", tt.level, p.Connected(), tt.connected)
		}
		if p.Low() != tt.low {
			t.Errorf("level %d: Low() = %v, want %v", tt.level, p.Low(), tt.low)
		}
		if p.Disconnected() != tt.disconnected {
			t.Errorf("level %d: Disconnected() = %v, want %v", tt.level, p.Disconnected(), tt.disconnected)
		}
	}
}

func TestParseModelRoundTrip(t *testing.T) {
	all := []pods.Model{
		pods.ModelUnknown, pods.ModelAirPods1, pods.ModelAirPods2, pods.ModelAirPods3,
		pods.ModelAirPodsPro, pods.ModelAirPodsPro2, pods.ModelAirPodsPro3, pods.ModelAirPodsMax,
		pods.ModelPowerbeatsPro, pods.ModelBeatsX, pods.ModelBeatsFlex, pods.ModelBeatsSolo3,
		pods.ModelBeatsStudio3, pods.ModelPowerbeats3,
	}
	for _, m := range all {
		if got := pods.ParseModel(m.String()); got != m {
			t.Errorf("ParseModel(%q) = %v, want %v", m.String(), got, m)
		}
	}
	if got := pods.ParseModel("nonsense"); got != pods.ModelUnknown {
		t.Errorf("ParseModel(nonsense) = %v, want ModelUnknown", got)
	}
}

func TestDisplayName(t *testing.T) {
	cases := map[pods.Model]string{
		pods.ModelAirPods1:      "AirPods (1st gen)",
		pods.ModelAirPodsPro:    "AirPods Pro",
		pods.ModelAirPodsPro2:   "AirPods Pro 2",
		pods.ModelAirPodsMax:    "AirPods Max",
		pods.ModelPowerbeatsPro: "Powerbeats Pro",
		pods.ModelBeatsStudio3:  "Beats Studio 3",
		pods.ModelUnknown:       "AirPods",
		pods.Model(999):         "AirPods",
	}
	for m, want := range cases {
		if got := pods.DisplayName(m); got != want {
			t.Errorf("DisplayName(%v) = %q, want %q", m, got, want)
		}
	}
}

func TestModelString(t *testing.T) {
	tests := []struct {
		model pods.Model
		want  string
	}{
		{pods.ModelUnknown, "unknown"},
		{pods.ModelAirPods1, "airpods1"},
		{pods.ModelAirPods2, "airpods2"},
		{pods.ModelAirPods3, "airpods3"},
		{pods.ModelAirPodsPro, "airpodspro"},
		{pods.ModelAirPodsPro2, "airpodspro2"},
		{pods.ModelAirPodsPro3, "airpodspro3"},
		{pods.ModelAirPodsMax, "airpodsmax"},
		{pods.ModelPowerbeatsPro, "powerbeatspro"},
		{pods.ModelBeatsX, "beatsx"},
		{pods.ModelBeatsFlex, "beatsflex"},
		{pods.ModelBeatsSolo3, "beatssolo3"},
		{pods.ModelBeatsStudio3, "beatsstudio3"},
		{pods.ModelPowerbeats3, "powerbeats3"},
		{pods.Model(-1), "unknown"},  // out-of-range value falls back to "unknown"
		{pods.Model(999), "unknown"}, // out-of-range value falls back to "unknown"
	}
	for _, tt := range tests {
		if got := tt.model.String(); got != tt.want {
			t.Errorf("Model(%d).String() = %q, want %q", tt.model, got, tt.want)
		}
	}
}
