package converter

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"protracker-go/engine"
	"protracker-go/mod"
)

// buildTestModule builds a minimal PTModule with one pattern, one sample,
// and a single note (C-2 on channel 0) so the renderer has something to mix.
func buildTestModule() *mod.PTModule {
	// 8-bit sine wave sample, 256 bytes
	sampleData := make([]byte, 256)
	for i := range sampleData {
		sampleData[i] = byte(int8(math.Sin(2*math.Pi*float64(i)/256.0) * 64))
	}

	m := &mod.PTModule{
		SongName:         "test",
		Variant:          "M.K.",
		NumberOfSamples:  1,
		SongLength:       1,
		SongPositions:    []byte{0, 0}, // one entry, pattern 0
		NumberOfPatterns: 1,
		SampleData: []mod.SampleData{
			{
				Name:         "sine",
				Length:       uint16(len(sampleData)),
				Volume:       64,
				RepeatStart:  0,
				RepeatLength: uint16(len(sampleData)),
				Data:         sampleData,
			},
		},
	}

	// Pattern 0: trigger C-2 (period 428) on channel 0 at row 0
	// All other rows/channels are silent
	m.Patterns = make([]mod.Pattern, 1)
	m.Patterns[0].Data[0][0] = mod.Note{
		Value:        428, // C-2
		SampleNumber: 1,
	}

	return m
}

func TestConvert_WAVHeader(t *testing.T) {
	m := buildTestModule()
	conv := NewMod2Wav(Stereo, 50)

	wav, err := conv.Convert(m)
	if err != nil {
		t.Fatalf("Convert returned error: %v", err)
	}

	if len(wav) < 44 {
		t.Fatalf("WAV too short: %d bytes", len(wav))
	}

	// Check RIFF magic
	if !bytes.Equal(wav[0:4], []byte("RIFF")) {
		t.Errorf("missing RIFF header: got %q", wav[0:4])
	}
	// Check WAVE magic
	if !bytes.Equal(wav[8:12], []byte("WAVE")) {
		t.Errorf("missing WAVE marker: got %q", wav[8:12])
	}
	// Check fmt sub-chunk
	if !bytes.Equal(wav[12:16], []byte("fmt ")) {
		t.Errorf("missing fmt chunk: got %q", wav[12:16])
	}
	// Check data sub-chunk marker
	if !bytes.Equal(wav[36:40], []byte("data")) {
		t.Errorf("missing data chunk: got %q", wav[36:40])
	}

	// Verify data chunk size matches actual payload
	dataSize := binary.LittleEndian.Uint32(wav[40:44])
	if int(dataSize) != len(wav)-44 {
		t.Errorf("data chunk size mismatch: header says %d, actual %d", dataSize, len(wav)-44)
	}
}

func TestConvert_Duration(t *testing.T) {
	m := buildTestModule()
	conv := NewMod2Wav(Stereo, 50)

	wav, err := conv.Convert(m)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	// Expected: 1 position × 64 rows × 6 ticks × 882 samples × 2 ch × 2 bytes = 1,354,752 bytes + 44 header
	// samplesPerTick = 44100*60/(125*24) = 882
	expectedSamples := 64 * engine.DefaultSpeed * engine.CalcTickSamples(engine.DefaultBPM)
	expectedBytes := 44 + expectedSamples*2*2 // stereo, 16-bit

	// Allow 1% tolerance for rounding
	tolerance := expectedBytes / 100
	actual := len(wav)
	if actual < expectedBytes-tolerance || actual > expectedBytes+tolerance {
		t.Errorf("unexpected WAV size: got %d, want ~%d (±%d)", actual, expectedBytes, tolerance)
	}
}

func TestConvert_Mono(t *testing.T) {
	m := buildTestModule()
	conv := NewMod2Wav(Mono, 0)

	wav, err := conv.Convert(m)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	// Mono: 1 channel in header
	numChannels := binary.LittleEndian.Uint16(wav[22:24])
	if numChannels != 1 {
		t.Errorf("expected 1 channel for Mono, got %d", numChannels)
	}
}

func TestConvert_Stereo(t *testing.T) {
	m := buildTestModule()
	conv := NewMod2Wav(Stereo, 100)

	wav, err := conv.Convert(m)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	numChannels := binary.LittleEndian.Uint16(wav[22:24])
	if numChannels != 2 {
		t.Errorf("expected 2 channels for Stereo, got %d", numChannels)
	}
}

// TestE6x_PatternLoop verifies that E6x causes rows to repeat the correct
// number of times, producing a longer WAV than a module without the loop.
func TestE6x_PatternLoop(t *testing.T) {
	// Base module: 1 pattern, no loop
	base := buildTestModule()

	// Loop module: same pattern but with E60 on row 0 ch0 and E62 on row 1 ch0
	// This makes rows 0–1 play 3× (1 initial + 2 repeats) before continuing.
	looped := buildTestModule()
	looped.Patterns[0].Data[0][0] = mod.Note{
		Value:         428,
		SampleNumber:  1,
		EffectCommand: 0x0E,
		EffectData:    0x60, // E60 — set loop start
	}
	looped.Patterns[0].Data[1][0] = mod.Note{
		EffectCommand: 0x0E,
		EffectData:    0x62, // E62 — loop 2 times
	}

	conv := NewMod2Wav(Stereo, 50)

	wavBase, err := conv.Convert(base)
	if err != nil {
		t.Fatalf("base Convert: %v", err)
	}
	wavLoop, err := conv.Convert(looped)
	if err != nil {
		t.Fatalf("loop Convert: %v", err)
	}

	// E62 = 2 extra replays of the 2-row loop body (rows 0+1).
	// 2 replays × 2 rows × 6 ticks/row = 24 extra ticks.
	extraTicks := 2 * 2 * engine.DefaultSpeed
	extraBytes := extraTicks * engine.CalcTickSamples(engine.DefaultBPM) * 2 * 2
	diff := len(wavLoop) - len(wavBase)
	tolerance := extraBytes / 20 // 5%

	if diff < extraBytes-tolerance || diff > extraBytes+tolerance {
		t.Errorf("E6x loop produced wrong length delta: got %d bytes extra, want ~%d", diff, extraBytes)
	}
}

func TestConvert_NotSilent(t *testing.T) {
	m := buildTestModule()
	conv := NewMod2Wav(Stereo, 50)

	wav, err := conv.Convert(m)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	// Scan PCM data for at least one non-zero sample
	found := false
	for i := 44; i+1 < len(wav); i += 2 {
		sample := int16(binary.LittleEndian.Uint16(wav[i : i+2]))
		if sample != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("all PCM samples are zero — audio is silent")
	}
}
