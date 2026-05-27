package converter

import (
	"bytes"
	"encoding/binary"
	"math"

	"protracker-go/engine"
	"protracker-go/mod"
)

type ChannelNum int

const (
	Mono ChannelNum = iota + 1
	Stereo
)

type Mod2Wav struct {
	numberOfChannels ChannelNum // 1 - mono, 2 stereo
	stereoSeparation int        // 0 = full mix, 100 = fully separated
}

func NewMod2Wav(chNum ChannelNum, stereoSep int) *Mod2Wav {
	return &Mod2Wav{
		numberOfChannels: chNum,
		stereoSeparation: stereoSep,
	}
}

// Convert renders a PTModule to WAV bytes (44100 Hz, 16-bit PCM).
func (m *Mod2Wav) Convert(module *mod.PTModule) ([]byte, error) {
	r := engine.NewReplayerState(module)

	var pcm []int16

	for !r.Done {
		floats := engine.RenderTick(r)
		// floats is stereo interleaved: [L0, R0, L1, R1, ...]
		for i := 0; i < len(floats); i += 2 {
			l, ri := applyMix(floats[i], floats[i+1], m.stereoSeparation, m.numberOfChannels)
			pcm = append(pcm, floatToInt16(l))
			if m.numberOfChannels == Stereo {
				pcm = append(pcm, floatToInt16(ri))
			}
		}
	}

	return encodeWAV(pcm, m.numberOfChannels, int(engine.OutputRate)), nil
}

// applyMix applies stereo separation and returns (left, right) normalised floats.
// With Mono, right is always 0.
func applyMix(left, right float64, sep int, ch ChannelNum) (float64, float64) {
	// Normalise: 4 voices max, each up to 1.0 → divide by 4
	left /= 4.0
	right /= 4.0

	if ch == Mono {
		return (left + right) * 0.5, 0
	}

	// Mid/side stereo separation
	mid := (left + right) * 0.5
	side := (left - right) * (float64(sep) / 100.0) * 0.5
	return mid + side, mid - side
}

// floatToInt16 clamps a [-1.0, 1.0] float to int16 range.
func floatToInt16(f float64) int16 {
	f = math.Max(-1.0, math.Min(1.0, f))
	return int16(f * math.MaxInt16)
}

// encodeWAV builds a standard RIFF/WAV file in memory.
func encodeWAV(samples []int16, ch ChannelNum, sampleRate int) []byte {
	numChannels := uint16(ch)
	bitsPerSample := uint16(16)
	blockAlign := numChannels * bitsPerSample / 8
	byteRate := uint32(sampleRate) * uint32(blockAlign)

	dataSize := uint32(len(samples)) * uint32(bitsPerSample/8)
	riffSize := 36 + dataSize // bytes after the "RIFF" + size fields

	buf := &bytes.Buffer{}

	// RIFF chunk
	buf.WriteString("RIFF")
	binary.Write(buf, binary.LittleEndian, riffSize)
	buf.WriteString("WAVE")

	// fmt sub-chunk
	buf.WriteString("fmt ")
	binary.Write(buf, binary.LittleEndian, uint32(16)) // sub-chunk size
	binary.Write(buf, binary.LittleEndian, uint16(1))  // PCM format
	binary.Write(buf, binary.LittleEndian, numChannels)
	binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	binary.Write(buf, binary.LittleEndian, byteRate)
	binary.Write(buf, binary.LittleEndian, blockAlign)
	binary.Write(buf, binary.LittleEndian, bitsPerSample)

	// data sub-chunk
	buf.WriteString("data")
	binary.Write(buf, binary.LittleEndian, dataSize)
	binary.Write(buf, binary.LittleEndian, samples)

	return buf.Bytes()
}
