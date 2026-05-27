package engine

import (
	"math"

	"protracker-go/mod"
)

const (
	paulaPalClk = 3_546_895.0 // Amiga PAL master clock Hz
	ciaPalClk   = 709_379.0   // Amiga PAL CIA clock Hz
	ciaPalBase  = 1_773_447   // integer: ciaPalClk * 2.5, used for BPM→period
	minPeriod   = 113
	maxPeriod   = 907
	maxVolume   = 64

	// OutputRate Exported for use by converter and tests.
	OutputRate   = 44_100.0 // render/WAV sample rate Hz
	DefaultBPM   = 125
	DefaultSpeed = 6
)

// sineTable is a 64-entry sine LUT scaled to 0–255, used for vibrato.
var sineTable = [64]int{
	0, 24, 49, 74, 97, 120, 141, 161,
	180, 197, 212, 224, 235, 244, 250, 253,
	255, 253, 250, 244, 235, 224, 212, 197,
	180, 161, 141, 120, 97, 74, 49, 24,
	0, -24, -49, -74, -97, -120, -141, -161,
	-180, -197, -212, -224, -235, -244, -250, -253,
	-255, -253, -250, -244, -235, -224, -212, -197,
	-180, -161, -141, -120, -97, -74, -49, -24,
}

// voiceState holds all per-channel playback state.
type voiceState struct {
	sample     *mod.SampleData
	volume     float64 // 0.0–1.0
	period     uint16  // current Paula period (may be modified by effects)
	basePeriod uint16  // period at note trigger (used by arpeggio/vibrato)
	phase      float64 // fractional sample read position
	delta      float64 // phase increment per output sample
	active     bool    // voice producing output

	// portamento
	portaTarget    uint16
	portaSpeed     uint8 // 3xx tone porta speed
	portaUpSpeed   uint8 // 1xx memory
	portaDownSpeed uint8 // 2xx memory

	// vibrato
	vibratoPos   uint8
	vibratoSpeed uint8
	vibratoDepth uint8

	// arpeggio
	arpeggioHi uint8 // high nibble of 0xx data
	arpeggioLo uint8 // low  nibble of 0xx data

	// sample offset (9xx)
	sampleOffset uint16

	// note delay (EDx)
	delayTick int // EDx: trigger note on this tick (-1 = inactive)

	// saved note for note-delay
	pendingNote *mod.Note

	// volume slide carry (Axx)
	volSlideUp   float64
	volSlideDown float64

	// repeat state
	repeatActive bool
}

// ReplayerState is the full state of the offline renderer.
type ReplayerState struct {
	module *mod.PTModule

	speed int // ticks per row
	bpm   int // current BPM

	tick int // tick within current row (0 … speed-1)
	row  int // row within current pattern (0 … 63)
	pos  int // position in song order (0 … SongLength-1)

	// CIA-precise tick timing: non-integer samples/tick accumulated fractionally.
	tickSampleFloat float64 // CIA-derived samples per tick (float)
	tickSampleAccum float64 // fractional carry between ticks

	voices [4]voiceState

	// pattern flow control — resolved at row advance
	patternBreak bool
	breakRow     int
	jumpPos      int // -1 = no jump

	patternDelay int // E.Ex: extra row repeats remaining

	// E6x pattern loop state
	loopStartRow    int  // row marked by E60; resets to 0 on position advance
	loopCounter     int  // -1 = inactive; ≥0 = remaining iterations
	patternLoopJump bool // signals advanceRow to jump to loopStartRow

	Done bool

	// FilterEnabled toggles the Amiga hardware low-pass filter (~4.4 kHz cutoff).
	// Off by default; set before first RenderTick call.
	FilterEnabled bool
	filterL       float64 // one-pole IIR state, left channel
	filterR       float64 // one-pole IIR state, right channel
}

func NewReplayerState(m *mod.PTModule) *ReplayerState {
	r := &ReplayerState{
		module:          m,
		speed:           DefaultSpeed,
		bpm:             DefaultBPM,
		tickSampleFloat: calcTickSampleFloat(DefaultBPM),
		jumpPos:         -1,
	}
	for i := range r.voices {
		r.voices[i].delayTick = -1
	}
	r.loopCounter = -1
	return r
}

// CalcTickSamples returns an integer approximation of samples/tick.
// Used in tests for expected-duration math.
func CalcTickSamples(bpm int) int {
	return int(OutputRate*60) / (bpm * 24)
}

// calcTickSampleFloat returns the CIA-precise (fractional) samples per tick.
//
// The Amiga CIA chip runs at 709,379 Hz (PAL). BPM is converted to a CIA
// timer period via: ciaPeriod = 1,773,447 / bpm  (integer, matches hardware).
// The actual tick frequency is: ciaPalClk / ciaPeriod.
// Samples per tick = OutputRate / tickFreq.
func calcTickSampleFloat(bpm int) float64 {
	ciaPeriod := ciaPalBase / bpm // integer division matches Amiga hardware
	tickFreq := ciaPalClk / float64(ciaPeriod)
	return OutputRate / tickFreq
}

// calcDelta converts a Paula period value to a per-sample phase increment.
func calcDelta(period uint16) float64 {
	if period == 0 {
		return 0
	}
	return paulaPalClk / (float64(period) * OutputRate)
}

// RenderTick produces one tick's worth of stereo float64 samples (interleaved L,R).
// It reads a new row on tick 0, updates effects on tick>0, then mixes all voices.
func RenderTick(r *ReplayerState) []float64 {
	patIdx := int(r.module.SongPositions[r.pos])
	var row [4]mod.Note
	if patIdx < len(r.module.Patterns) {
		row = r.module.Patterns[patIdx].Data[r.row]
	}

	if r.tick == 0 {
		readRow(r)
	} else {
		for ch := 0; ch < 4; ch++ {
			v := &r.voices[ch]
			n := row[ch]
			// Note delay: trigger on the right tick
			if v.delayTick >= 0 && r.tick == v.delayTick && v.pendingNote != nil {
				triggerNote(v, *v.pendingNote, r.module.SampleData)
				v.active = v.sample != nil && len(v.sample.Data) > 0
			}
			applyEffectTickN(v, n, r.tick)
		}
	}

	// CIA-precise tick length: accumulate fractional samples and extract integer count.
	r.tickSampleAccum += r.tickSampleFloat
	n := int(r.tickSampleAccum)
	r.tickSampleAccum -= float64(n)

	out := make([]float64, n*2) // stereo interleaved

	for i := 0; i < n; i++ {
		var left, right float64

		for ch := 0; ch < 4; ch++ {
			v := &r.voices[ch]
			if !v.active || v.sample == nil || len(v.sample.Data) == 0 {
				continue
			}

			frac := v.phase - float64(int(v.phase)) // fractional position for interpolation
			pos := int(v.phase)

			// Handle repeat / end of sample
			if v.repeatActive {
				loopStart := int(v.sample.RepeatStart)
				loopEnd := loopStart + int(v.sample.RepeatLength)
				if pos >= loopEnd {
					excess := pos - loopEnd
					pos = loopStart + (excess % int(v.sample.RepeatLength))
					v.phase = float64(pos) + frac
				}
			} else {
				if pos >= len(v.sample.Data) {
					v.active = false
					continue
				}
			}

			if pos < 0 || pos >= len(v.sample.Data) {
				v.active = false
				continue
			}

			// Next sample position for linear interpolation (respects loop boundary).
			var nextPos int
			if v.repeatActive {
				loopStart := int(v.sample.RepeatStart)
				loopEnd := loopStart + int(v.sample.RepeatLength)
				nextPos = pos + 1
				if nextPos >= loopEnd {
					nextPos = loopStart
				}
			} else {
				nextPos = pos + 1
				if nextPos >= len(v.sample.Data) {
					nextPos = pos // clamp at end of one-shot sample
				}
			}

			// Linear interpolation between adjacent samples reduces aliasing
			// when delta > 1 (high pitch / fast playback).
			s0 := float64(int8(v.sample.Data[pos])) / 128.0
			s1 := float64(int8(v.sample.Data[nextPos])) / 128.0
			raw := s0 + frac*(s1-s0)
			sample := raw * v.volume

			// Amiga hardware panning: ch 0,2 → left; ch 1,3 → right
			if ch == 0 || ch == 2 {
				left += sample
			} else {
				right += sample
			}

			v.phase += v.delta
		}

		// Amiga hardware low-pass filter (~4.4 kHz cutoff, one-pole IIR).
		// Coefficient: exp(-2π * 4413 / 44100) ≈ 0.5338
		if r.FilterEnabled {
			const a = 0.5338
			left = (1-a)*left + a*r.filterL
			right = (1-a)*right + a*r.filterR
			r.filterL = left
			r.filterR = right
		}

		out[i*2] = left
		out[i*2+1] = right
	}

	// Advance tick counter; advance row when speed boundary reached
	r.tick++
	if r.tick >= r.speed {
		r.tick = 0
		advanceRow(r)
	}

	return out
}

// clampPeriod constrains a period to the legal Amiga range.
func clampPeriod(p uint16) uint16 {
	if p < minPeriod {
		return minPeriod
	}
	if p > maxPeriod {
		return maxPeriod
	}
	return p
}

// readRow reads the current row from the current pattern and triggers
// notes + tick-0 effects for all four channels.
func readRow(r *ReplayerState) {
	patIdx := int(r.module.SongPositions[r.pos])
	if patIdx >= len(r.module.Patterns) {
		return
	}
	row := r.module.Patterns[patIdx].Data[r.row]
	for ch := 0; ch < 4; ch++ {
		n := row[ch]
		v := &r.voices[ch]
		// Reset per-row transient state
		v.delayTick = -1
		v.pendingNote = nil

		// Pre-apply 9xx so triggerNote sees the correct sample offset.
		if n.EffectCommand == 0x09 && n.EffectData != 0 {
			v.sampleOffset = uint16(n.EffectData) * 256
		}

		if n.EffectCommand == 0x0E && (n.EffectData>>4) == 0x0D {
			// Note delay: save note, will trigger on the right tick
			v.pendingNote = &n
			applyEffectTick0(v, n, r)
		} else {
			triggerNote(v, n, r.module.SampleData)
			applyEffectTick0(v, n, r)
		}
	}
}

// advanceRow moves to the next row, handling pattern loop, break/jump and song end.
func advanceRow(r *ReplayerState) {
	if r.patternDelay > 0 {
		r.patternDelay--
		return // repeat current row without advancing
	}

	// E6x loop takes priority over pattern break/jump
	if r.patternLoopJump {
		r.patternLoopJump = false
		r.row = r.loopStartRow
		return // stay in same pattern position
	}

	if r.patternBreak || r.jumpPos >= 0 {
		if r.jumpPos >= 0 {
			r.pos = r.jumpPos
		} else {
			r.pos++
		}
		r.row = r.breakRow
		r.patternBreak = false
		r.breakRow = 0
		r.jumpPos = -1
		// Reset loop state — we've moved to a new position
		r.loopStartRow = 0
		r.loopCounter = -1
	} else {
		r.row++
		if r.row >= 64 {
			r.row = 0
			r.pos++
			// Reset loop state — new pattern position
			r.loopStartRow = 0
			r.loopCounter = -1
		}
	}

	if r.pos >= int(r.module.SongLength) {
		r.Done = true
	}
}

// applyFineTune adjusts a Paula period by a sample's fine-tune nibble.
//
// Fine-tune is a signed 4-bit value stored as 0–15:
//
//	0       = no adjustment
//	1–7     = raise pitch by 1/8 to 7/8 semitone (lower period)
//	8–15    = lower pitch by 8/8 to 1/8 semitone (higher period), i.e. -8 to -1
//
// Formula: period * 2^(-ft/96), where ft is the signed value.
// Positive ft raises pitch → smaller period; negative ft lowers pitch → larger period.
func applyFineTune(period uint16, fineTune byte) uint16 {
	ft := int(fineTune & 0x0F)
	if ft == 0 {
		return period
	}
	if ft > 7 {
		ft -= 16 // 8→-8, 9→-7, …, 15→-1
	}
	adjusted := float64(period) * math.Pow(2.0, float64(-ft)/96.0)
	return clampPeriod(uint16(math.Round(adjusted)))
}

// triggerNote sets up a voice from a parsed note at tick 0.
// Rules per ProTracker spec:
//   - Value==0, SampleNumber==0 → keep playing, only apply effect
//   - Value==0, SampleNumber>0  → change sample/volume, keep period
//   - Value>0,  SampleNumber==0 → retrigger at new period, keep sample
//   - Value>0,  SampleNumber>0  → full trigger
//   - Effect 3xx (tone porta)   → set portaTarget but do NOT reset phase
func triggerNote(v *voiceState, n mod.Note, samples []mod.SampleData) {
	isPorta := n.EffectCommand == 0x03

	// Resolve sample
	if n.SampleNumber > 0 && int(n.SampleNumber) <= len(samples) {
		s := &samples[n.SampleNumber-1]
		v.sample = s
		vol := s.Volume
		if vol > maxVolume {
			vol = maxVolume
		}
		v.volume = float64(vol) / float64(maxVolume)
		// RepeatLength <= 2 means no real loop in Amiga convention
		v.repeatActive = s.RepeatLength > 2
	}

	if n.Value > 0 {
		period := clampPeriod(n.Value)
		if v.sample != nil {
			period = applyFineTune(period, v.sample.FineTune)
		}

		if isPorta {
			// Tone portamento: slide toward this period, do not retrigger
			v.portaTarget = period
		} else {
			v.period = period
			v.basePeriod = period
			v.phase = float64(v.sampleOffset) // 9xx applies here
			v.sampleOffset = 0
			v.delta = calcDelta(period)
			v.active = v.sample != nil && len(v.sample.Data) > 0
			// Reset vibrato/arpeggio accumulators on new note
			v.vibratoPos = 0
			v.arpeggioHi = 0
			v.arpeggioLo = 0
		}
	}

	if v.sample == nil {
		v.active = false
	}
}
