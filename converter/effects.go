package converter

import "protracker-go/mod"

// applyEffectTick0 handles effects that take action on tick 0 (note row read).
// Called after triggerNote so volume/period from the note are already set.
func applyEffectTick0(v *voiceState, n mod.Note, r *replayerState) {
	cmd := n.EffectCommand
	data := n.EffectData
	hi := data >> 4
	lo := data & 0x0F

	switch cmd {
	case 0x00: // Arpeggio — store semitone offsets, base already set
		v.arpeggioHi = hi
		v.arpeggioLo = lo

	case 0x03: // Tone portamento — speed; target set in triggerNote
		if data != 0 {
			v.portaSpeed = data
		}

	case 0x04: // Vibrato — set speed/depth
		if hi != 0 {
			v.vibratoSpeed = hi
		}
		if lo != 0 {
			v.vibratoDepth = lo
		}

	case 0x09: // Sample offset
		v.sampleOffset = uint16(data) * 256
		// Phase already applied in triggerNote; if note.Value==0 apply now
		if n.Value == 0 {
			v.phase = float64(v.sampleOffset)
		}

	case 0x0A: // Volume slide — store rates (applied tick>0)
		v.volSlideUp = float64(hi) / float64(maxVolume)
		v.volSlideDown = float64(lo) / float64(maxVolume)

	case 0x0B: // Position jump
		r.jumpPos = int(data)
		r.patternBreak = true
		r.breakRow = 0

	case 0x0C: // Set volume
		vol := data
		if vol > maxVolume {
			vol = maxVolume
		}
		v.volume = float64(vol) / float64(maxVolume)

	case 0x0D: // Pattern break — row = hi*10 + lo
		r.patternBreak = true
		row := int(hi)*10 + int(lo)
		if row > 63 {
			row = 0
		}
		if row > r.breakRow {
			r.breakRow = row
		}

	case 0x0F: // Set speed or BPM
		if data < 0x20 {
			r.speed = int(data)
			if r.speed == 0 {
				r.speed = 1
			}
		} else {
			r.bpm = int(data)
			r.tickSamples = calcTickSamples(r.bpm)
		}

	case 0x0E: // Extended effects
		applyExtendedTick0(v, hi, lo, r)
	}
}

func applyExtendedTick0(v *voiceState, hi, lo uint8, r *replayerState) {
	switch hi {
	case 0x06: // E6x — pattern loop
		if lo == 0 {
			// E60: mark current row as loop start
			r.loopStartRow = r.row
		} else {
			// E6x: init counter on first encounter, then count down
			if r.loopCounter < 0 {
				r.loopCounter = int(lo)
			}
			if r.loopCounter > 0 {
				r.loopCounter--
				r.patternLoopJump = true
			} else {
				// Counter expired — fall through, reset for next time
				r.loopCounter = -1
			}
		}

	case 0x09: // E9x — retrigger note every x ticks
		// handled in tick>0

	case 0x0A: // EAx — fine volume slide up
		v.volume += float64(lo) / float64(maxVolume)
		if v.volume > 1.0 {
			v.volume = 1.0
		}

	case 0x0B: // EBx — fine volume slide down
		v.volume -= float64(lo) / float64(maxVolume)
		if v.volume < 0 {
			v.volume = 0
		}

	case 0x0C: // ECx — note cut on tick x
		v.cutTick = int(lo)

	case 0x0D: // EDx — note delay
		v.delayTick = int(lo)
		// Undo trigger; will fire on the right tick
		v.active = false

	case 0x0E: // EEx — pattern delay
		r.patternDelay = int(lo)
	}
}

// applyEffectTickN handles effects that update on tick > 0.
func applyEffectTickN(v *voiceState, n mod.Note, tick int) {
	cmd := n.EffectCommand
	data := n.EffectData
	hi := data >> 4
	lo := data & 0x0F

	switch cmd {
	case 0x00: // Arpeggio
		if data == 0 {
			break
		}
		phase := tick % 3
		switch phase {
		case 0:
			v.period = v.basePeriod
		case 1:
			v.period = semitoneUp(v.basePeriod, v.arpeggioHi)
		case 2:
			v.period = semitoneUp(v.basePeriod, v.arpeggioLo)
		}
		v.delta = calcDelta(v.period)

	case 0x01: // Portamento up (period decreases = pitch rises)
		if v.period > minPeriod {
			p := int(v.period) - int(data)
			if p < minPeriod {
				p = minPeriod
			}
			v.period = uint16(p)
			v.delta = calcDelta(v.period)
		}

	case 0x02: // Portamento down
		if v.period < maxPeriod {
			p := int(v.period) + int(data)
			if p > maxPeriod {
				p = maxPeriod
			}
			v.period = uint16(p)
			v.delta = calcDelta(v.period)
		}

	case 0x03: // Tone portamento
		if v.portaTarget == 0 {
			break
		}
		speed := int(v.portaSpeed)
		if v.period < v.portaTarget {
			p := int(v.period) + speed
			if p > int(v.portaTarget) {
				p = int(v.portaTarget)
			}
			v.period = uint16(p)
		} else if v.period > v.portaTarget {
			p := int(v.period) - speed
			if p < int(v.portaTarget) {
				p = int(v.portaTarget)
			}
			v.period = uint16(p)
		}
		v.delta = calcDelta(v.period)

	case 0x04: // Vibrato — oscillate period using sine table
		offset := int(sineTable[v.vibratoPos&63]) * int(v.vibratoDepth) / 128
		period := int(v.basePeriod) + offset
		if period < minPeriod {
			period = minPeriod
		}
		v.period = uint16(period)
		v.delta = calcDelta(v.period)
		v.vibratoPos = (v.vibratoPos + v.vibratoSpeed) & 63

	case 0x0A: // Volume slide
		if hi > 0 {
			v.volume += v.volSlideUp
		} else {
			v.volume -= v.volSlideDown
		}
		if v.volume > 1.0 {
			v.volume = 1.0
		}
		if v.volume < 0 {
			v.volume = 0
		}

	case 0x0E: // Extended tick-N effects
		applyExtendedTickN(v, hi, lo, tick, n)
	}
}

func applyExtendedTickN(v *voiceState, hi, lo uint8, tick int, n mod.Note) {
	switch hi {
	case 0x09: // E9x — retrigger every x ticks
		if lo > 0 && tick%int(lo) == 0 {
			v.phase = 0
			v.active = v.sample != nil && len(v.sample.Data) > 0
		}

	case 0x0C: // ECx — note cut
		if tick == int(lo) {
			v.volume = 0
		}

	case 0x0D: // EDx — note delay
		if tick == int(lo) {
			triggerNote(v, n, nil) // sample already resolved at tick 0
			v.active = v.sample != nil && len(v.sample.Data) > 0
		}
	}
}

// semitoneUp shifts a period up by n semitones using the period lookup table.
// Falls back to the base period if the offset is out of range.
func semitoneUp(basePeriod uint16, semitones uint8) uint16 {
	if semitones == 0 {
		return basePeriod
	}
	for i, p := range periodTable {
		if p == basePeriod {
			idx := i - int(semitones)
			if idx < 0 {
				idx = 0
			}
			return periodTable[idx]
		}
	}
	return basePeriod
}

// periodTable mirrors mod.periodLookup for use within this package.
var periodTable = []uint16{
	1712, 1616, 1524, 1440, 1356, 1280, 1208, 1140, 1076, 1016, 960, 907,
	856, 808, 762, 720, 678, 640, 604, 570, 538, 508, 480, 453,
	428, 404, 381, 360, 339, 320, 302, 285, 269, 254, 240, 226,
	214, 202, 190, 180, 170, 160, 151, 143, 135, 127, 120, 113,
	107, 101, 95, 90, 85, 80, 75, 71, 67, 63, 60, 56,
}
