package converter

import "protracker-go/mod"

// applyEffectTick0 handles effects that take action on tick 0 (note row read).
// Called after triggerNote so volume/period from the note are already set.
func applyEffectTick0(v *voiceState, n mod.Note, r *ReplayerState) {
	cmd := n.EffectCommand
	data := n.EffectData
	hi := data >> 4
	lo := data & 0x0F

	switch cmd {
	case 0x00: // Arpeggio — store semitone offsets, base already set
		v.arpeggioHi = hi
		v.arpeggioLo = lo

	case 0x03: // Tone portamento — store speed; target set in triggerNote
		if data != 0 {
			v.portaSpeed = data
		}

	case 0x04: // Vibrato — set speed/depth (0 = keep previous)
		if hi != 0 {
			v.vibratoSpeed = hi
		}
		if lo != 0 {
			v.vibratoDepth = lo
		}

	case 0x05: // Tone portamento + volume slide
		// Portamento: keep previous speed (no speed param in this command)
		// Volume slide: hi=up, lo=down; 0 = keep previous rates
		if data != 0 {
			v.volSlideUp = float64(hi) / float64(maxVolume)
			v.volSlideDown = float64(lo) / float64(maxVolume)
		}

	case 0x06: // Vibrato + volume slide
		// Vibrato: keep previous speed/depth
		// Volume slide: same memory rule as 0x0A
		if data != 0 {
			v.volSlideUp = float64(hi) / float64(maxVolume)
			v.volSlideDown = float64(lo) / float64(maxVolume)
		}

	case 0x09: // Sample offset — already pre-applied in readRow; handle no-note case
		if n.Value == 0 && n.EffectData != 0 {
			v.phase = float64(uint16(data) * 256)
		}

	case 0x0A: // Volume slide — store rates; 0 means continue previous
		if data != 0 {
			v.volSlideUp = float64(hi) / float64(maxVolume)
			v.volSlideDown = float64(lo) / float64(maxVolume)
		}

	case 0x0B: // Position jump
		pos := int(data)
		if pos >= int(r.module.SongLength) {
			pos = int(r.module.SongLength) - 1
		}
		r.jumpPos = pos
		r.patternBreak = true
		r.breakRow = 0

	case 0x0C: // Set volume
		vol := data
		if vol > maxVolume {
			vol = maxVolume
		}
		v.volume = float64(vol) / float64(maxVolume)

	case 0x0D: // Pattern break — row = hi*10 + lo (BCD decimal), last channel wins
		r.patternBreak = true
		row := int(hi)*10 + int(lo)
		if row > 63 {
			row = 0
		}
		r.breakRow = row

	case 0x0F: // Set speed or BPM
		if data < 0x20 {
			r.speed = int(data)
			if r.speed == 0 {
				r.speed = 1
			}
		} else {
			r.bpm = int(data)
			r.tickSampleFloat = calcTickSampleFloat(r.bpm)
			// Accumulator carries over — avoids a sample-count glitch at transition
		}

	case 0x0E: // Extended effects
		applyExtendedTick0(v, hi, lo, r)
	}
}

func applyExtendedTick0(v *voiceState, hi, lo uint8, r *ReplayerState) {
	switch hi {
	case 0x01: // E1x — fine portamento up (applied once, tick 0 only)
		p := int(v.period) - int(lo)
		if p < minPeriod {
			p = minPeriod
		}
		v.period = uint16(p)
		v.delta = calcDelta(v.period)

	case 0x02: // E2x — fine portamento down (applied once, tick 0 only)
		p := int(v.period) + int(lo)
		if p > maxPeriod {
			p = maxPeriod
		}
		v.period = uint16(p)
		v.delta = calcDelta(v.period)

	case 0x06: // E6x — pattern loop
		if lo == 0 {
			r.loopStartRow = r.row
		} else {
			if r.loopCounter < 0 {
				r.loopCounter = int(lo)
			}
			if r.loopCounter > 0 {
				r.loopCounter--
				r.patternLoopJump = true
			} else {
				r.loopCounter = -1
			}
		}

	case 0x09: // E9x — retrigger: handled tick>0

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

	case 0x0C: // ECx — note cut on tick x (checked in applyEffectTickN)

	case 0x0D: // EDx — note delay (checked in RenderTick + applyEffectTickN)
		v.delayTick = int(lo)
		v.active = false

	case 0x0E: // EEx — pattern delay
		r.patternDelay = int(lo)
	}
}

// applyEffectTickN handles effects that update on ticks > 0.
func applyEffectTickN(v *voiceState, n mod.Note, tick int) {
	cmd := n.EffectCommand
	data := n.EffectData
	hi := data >> 4
	lo := data & 0x0F

	switch cmd {
	case 0x00: // Arpeggio — cycle base / +hi / +lo semitones
		if data == 0 {
			break
		}
		switch tick % 3 {
		case 0:
			v.period = v.basePeriod
		case 1:
			v.period = semitoneUp(v.basePeriod, v.arpeggioHi)
		case 2:
			v.period = semitoneUp(v.basePeriod, v.arpeggioLo)
		}
		v.delta = calcDelta(v.period)

	case 0x01: // Portamento up — pitch rises (period decreases); memory: 0 = reuse speed
		if data != 0 {
			v.portaUpSpeed = data
		}
		if v.portaUpSpeed > 0 {
			p := int(v.period) - int(v.portaUpSpeed)
			if p < minPeriod {
				p = minPeriod
			}
			v.period = uint16(p)
			v.delta = calcDelta(v.period)
		}

	case 0x02: // Portamento down — pitch falls (period increases); memory: 0 = reuse speed
		if data != 0 {
			v.portaDownSpeed = data
		}
		if v.portaDownSpeed > 0 {
			p := int(v.period) + int(v.portaDownSpeed)
			if p > maxPeriod {
				p = maxPeriod
			}
			v.period = uint16(p)
			v.delta = calcDelta(v.period)
		}

	case 0x03: // Tone portamento — slide period toward portaTarget
		tonePortamento(v)

	case 0x04: // Vibrato — oscillate period with sine LFO
		vibrato(v)

	case 0x05: // Tone portamento + volume slide
		tonePortamento(v)
		if data != 0 {
			volumeSlide(v, hi)
		}

	case 0x06: // Vibrato + volume slide
		vibrato(v)
		if data != 0 {
			volumeSlide(v, hi)
		}

	case 0x0A: // Volume slide — A00 = no slide (no memory in PT2)
		if data != 0 {
			volumeSlide(v, hi)
		}

	case 0x0E: // Extended tick-N effects
		applyExtendedTickN(v, hi, lo, tick)
	}
}

// tonePortamento slides v.period toward v.portaTarget by v.portaSpeed.
func tonePortamento(v *voiceState) {
	if v.portaTarget == 0 {
		return
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
}

// vibrato oscillates v.period around v.basePeriod using the sine table.
func vibrato(v *voiceState) {
	offset := int(sineTable[v.vibratoPos&63]) * int(v.vibratoDepth) / 128
	period := int(v.basePeriod) + offset
	if period < minPeriod {
		period = minPeriod
	}
	if period > maxPeriod {
		period = maxPeriod
	}
	v.period = uint16(period)
	v.delta = calcDelta(v.period)
	v.vibratoPos = (v.vibratoPos + v.vibratoSpeed) & 63
}

// volumeSlide adjusts v.volume by stored slide rates. hi!=0 → slide up, else down.
func volumeSlide(v *voiceState, hi uint8) {
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
}

func applyExtendedTickN(v *voiceState, hi, lo uint8, tick int) {
	switch hi {
	case 0x09: // E9x — retrigger every x ticks
		if lo > 0 && tick%int(lo) == 0 {
			v.phase = 0
			v.active = v.sample != nil && len(v.sample.Data) > 0
		}

	case 0x0C: // ECx — note cut at tick x
		if tick == int(lo) {
			v.volume = 0
		}

		// EDx: handled entirely in RenderTick (has access to module samples)
	}
}

// semitoneUp returns the period n semitones higher than basePeriod.
// Higher pitch = lower period value = higher index in periodTable.
func semitoneUp(basePeriod uint16, semitones uint8) uint16 {
	if semitones == 0 {
		return basePeriod
	}
	for i, p := range mod.PeriodLookup {
		if p == basePeriod {
			idx := i + int(semitones)
			if idx >= len(mod.PeriodLookup) {
				idx = len(mod.PeriodLookup) - 1
			}
			return mod.PeriodLookup[idx]
		}
	}
	return basePeriod
}
