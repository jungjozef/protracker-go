# mod2wav Implementation Plan

## Goal
Render a parsed PTModule to a WAV file in memory. Fast (offline), not real-time.
Output: 44100 Hz, 16-bit PCM, mono or stereo with configurable separation.

## File Structure
```
converter/
  mod2wav.go      — Convert(), WAV encoder, mixer (exists, has skeleton)
  replayer.go     — new: ReplayerState, voiceState, tick engine
  effects.go      — new: per-effect update functions
```

---

## Steps

### Step 1 — Constants, voice state, replayer state structs
File: `converter/replayer.go`

- Constants: paulaPalClk, outputRate, defaultBPM, defaultSpeed, minPeriod, maxPeriod, maxVolume
- `voiceState` struct: sample ptr, volume (float64 0–1), period (uint16), phase (float64),
  delta (float64), active (bool), repeatActive (bool),
  portaTarget (uint16), portaSpeed (uint8),
  vibratoPos (uint8), vibratoSpeed (uint8), vibratoDepth (uint8),
  arpeggioBaseNote (uint16), sampleOffset (uint16),
  noteCut (bool), noteDelay (uint8), pendingNote (*mod.Note)
- `ReplayerState` struct: module ptr, speed, bpm, tick, row, pos,
  tickSamples (int), voices [4]voiceState,
  patternDelay (int),
  patternBreak (bool), breakRow (int), jumpPos (int), done (bool)
- `NewReplayerState(m *mod.PTModule) *ReplayerState` constructor: sets defaults

### Step 2 — Tick sample count + period→delta helpers
File: `converter/replayer.go`

- `calcTickSamples(bpm int) int`  →  `outputRate * 60 / (bpm * 24)`
- `calcDelta(period uint16) float64`  →  `paulaPalClk / (float64(period) * outputRate)`
- `clampPeriod(p uint16) uint16`  →  clamp [minPeriod, maxPeriod]
- Sine table for vibrato (64-entry, amplitude 0–255)

### Step 3 — Note trigger (tick 0 channel update)
File: `converter/replayer.go`

- `triggerNote(v *voiceState, n mod.Note, samples []mod.SampleData)`
- Sets period, resets phase (or applies 9xx sample offset), sets active
- Looks up sample by SampleNumber, sets volume from sample default
- If note.Value == 0 and no sample: keep playing (only effect)
- If note.Value == 0 and sample change: retrigger sample, keep period
- Porta (3xx): set portaTarget but do NOT reset phase

### Step 4 — Effect handlers (tick 0: set)
File: `converter/effects.go`

Implement for tick == 0:
- `Fxx`: xx < 0x20 → set speed; else set BPM, recompute tickSamples
- `Cxx`: set volume min(xx, 64) / 64.0
- `Bxx`: set jumpPos, patternBreak
- `Dxx`: set breakRow (hi*10 + lo), patternBreak
- `9xx`: sampleOffset = xx * 256; applied in triggerNote
- `Exx` extended:
  - `E6x`: set loop start / trigger loop
  - `EAx`: fine volume slide up
  - `EBx`: fine volume slide down
  - `ECx`: note cut on tick x
  - `EDx`: note delay — defer trigger to tick x

### Step 5 — Effect handlers (tick > 0: update)
File: `converter/effects.go`

Implement for tick > 0:
- `0xx` Arpeggio: cycle base / base+hi / base+lo periods (tick%3)
- `1xx` Porta up: period -= xx, clamp to minPeriod, recompute delta
- `2xx` Porta down: period += xx, clamp to maxPeriod, recompute delta
- `3xx` Tone porta: slide period toward portaTarget by portaSpeed, clamp
- `4xx` Vibrato: period offset = sineTable[vibratoPos] * depth / 128; advance pos
- `Axx` Volume slide: vol += hiNibble/64 - loNibble/64 per tick, clamp [0,1]
- `ECx` Note cut: zero volume when tick == cutTick
- `EDx` Note delay: trigger note when tick == delayTick

### Step 6 — Row reader + pattern sequencer
File: `converter/replayer.go`

- `readRow(r *ReplayerState)`: reads current row from current pattern,
  calls triggerNote + tick-0 effects for each channel
- `advanceRow(r *ReplayerState)`: increments row, handles pattern break/jump,
  wraps at SongLength → sets r.done = true
- `applyPatternDelay`: if E.Dx set, repeat current row N extra times

### Step 7 — Per-tick render loop
File: `converter/replayer.go`

- `RenderTick(r *ReplayerState) []float64`
  - if tick==0: call readRow
  - call updateEffects (tick>0 handlers) for each channel
  - inner loop for tickSamples iterations:
    - for each voice: advance phase, read sample byte (signed int8→float64)
    - handle repeat / silence
    - scale by volume
    - accumulate left (ch 0,2) and right (ch 1,3) float64 buffers
  - increment tick; if tick==speed: tick=0, advanceRow
  - return float64 stereo interleaved buffer

### Step 8 — Mixer + stereo separation
File: `converter/mod2wav.go`

- `mix(left, right float64, sep int, chNum ChannelNum) (l, r float64)`
  - mid  = (left + right) * 0.5
  - side = (left − right) * float64(sep) / 100.0 * 0.5
  - outL = mid + side
  - outR = mid − side
  - mono: return (left+right)*0.5, 0
- Normalise: divide each sample by 4.0 (4 voices max) before convert to int16
- Clamp to [-1.0, 1.0], scale to int16

### Step 9 — WAV encoder
File: `converter/mod2wav.go`

- `encodeWAV(samples []int16, ch ChannelNum, rate int) []byte`
- Write RIFF/WAV header (44 bytes) then raw int16 LE PCM
- Header fields: numChannels (1 or 2), sampleRate, byteRate, blockAlign, bitsPerSample
- Use `encoding/binary` LittleEndian throughout

### Step 10 — Wire Convert()
File: `converter/mod2wav.go`

```go
func (m *Mod2Wav) Convert(module *mod.PTModule) ([]byte, error) {
    r := NewReplayerState(module)
    var pcm []int16
    for !r.done {
        floats := RenderTick(r)
        for i := 0; i < len(floats); i += 2 {
            l, ri := mix(floats[i], floats[i+1], m.stereoSeparation, m.numberOfChannels)
            pcm = append(pcm, floatToInt16(l))
            if m.numberOfChannels == Stereo {
                pcm = append(pcm, floatToInt16(ri))
            }
        }
    }
    return encodeWAV(pcm, m.numberOfChannels, outputRate), nil
}
```

### Step 11 — Integration test
File: `converter/mod2wav_test.go`

- Load a real .mod file via LoadPTModule
- Call Convert(), check:
  - no error
  - output starts with "RIFF" and "WAVE"
  - data chunk length > 0
  - reasonable duration (within 10% of expected based on BPM/rows)

---

## Effect implementation priority
Must-have (most MODs): Fxx, Cxx, Axx, Dxx, Bxx, 1xx, 2xx, 3xx
Nice-to-have: 4xx, 0xx, 9xx, Exx extended
Skip for now: BLEP anti-aliasing (correctness first, quality later)

## Known simplifications vs pt2-clone
- No BLEP (band-limited step): expect some aliasing on sharp note transitions
- No CIA timer precision: tickSamples is integer-rounded
- No filter emulation (Amiga low-pass)
- Linear interpolation of sample data optional (add later)