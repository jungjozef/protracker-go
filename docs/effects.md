# ProTracker Effects

Implementation lives in `engine/effects.go`.

## Implemented

| Effect | Name | Tick-0 | Tick-N |
|--------|------|--------|--------|
| `0xx`  | Arpeggio | store hi/lo semitones | cycle base/+hi/+lo each tick |
| `1xx`  | Portamento up | store speed | period -= speed/tick, clamp |
| `2xx`  | Portamento down | store speed | period += speed/tick, clamp |
| `3xx`  | Tone portamento | set portaTarget, store speed | slide period toward target |
| `4xx`  | Vibrato | set speed/depth | oscillate period via sine LUT |
| `5xx`  | Tone porta + vol slide | init vol slide rates | porta + slide |
| `6xx`  | Vibrato + vol slide | init vol slide rates | vibrato + slide |
| `9xx`  | Sample offset | phase = xx*256 | — |
| `Axx`  | Volume slide | store up/down rates | vol += up or -= down |
| `Bxx`  | Position jump | set jumpPos | — |
| `Cxx`  | Set volume | vol = min(xx, 64)/64 | — |
| `Dxx`  | Pattern break | set breakRow (BCD hi*10+lo) | — |
| `E1x`  | Fine porta up | period -= x (once) | — |
| `E2x`  | Fine porta down | period += x (once) | — |
| `E6x`  | Pattern loop | E60: set start; E6x: loop x times | — |
| `E9x`  | Retrigger | — | reset phase every x ticks |
| `EAx`  | Fine vol slide up | vol += x/64 (once) | — |
| `EBx`  | Fine vol slide down | vol -= x/64 (once) | — |
| `ECx`  | Note cut | — | vol = 0 at tick x |
| `EDx`  | Note delay | defer trigger to tick x | trigger at tick x |
| `EEx`  | Pattern delay | repeat row x extra times | — |
| `Fxx`  | Set speed/BPM | xx<0x20 → speed; else BPM | — |

## Not implemented

- BLEP anti-aliasing (band-limited step)
- Amiga low-pass filter emulation
- Fine-tune (sample `FineTune` field parsed but not applied to delta)
