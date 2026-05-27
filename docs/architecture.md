# Architecture

## Package layout

```
protracker-go/           ← module "protracker-go"
  mod/                   ← data model: PTModule, Pattern, Note, SampleData
  loader/                ← binary MOD parser
  engine/                ← core render engine (format-agnostic)
  converter/             ← WAV output (depends on engine)
  main.go                ← CLI: --mode play|convert

player/                  ← module "protracker-player" (go.work sibling)
  player.go              ← real-time audio output via oto v3
```

## Data flow

```
loader.LoadPTModule(io.ReadSeeker)
  → *mod.PTModule

engine.NewReplayerState(*mod.PTModule)
  → *engine.ReplayerState

loop: engine.RenderTick(*ReplayerState)
  → []float64  stereo-interleaved samples (L0, R0, L1, R1, ...)

[offline]  converter.Mod2Wav.Convert → applyMix → encodeWAV → []byte (RIFF/WAV)
[realtime] player.ModReader.Read     → applyMix → int16 LE  → oto → DAC
```

## engine/

**`replayer.go`** — tick engine

- `ReplayerState` — full playback state (pos/row/tick, 4 voices, BPM, CIA accumulator)
- `RenderTick(r)` — produces one tick of stereo float64 samples
  - tick 0: `readRow()` — latches notes + tick-0 effects
  - tick N: `applyEffectTickN()` per voice
  - CIA-precise timing: fractional `tickSampleAccum` avoids integer drift
  - Amiga hardware panning: ch 0,2 → left; ch 1,3 → right
- Key constants: `paulaPalClk=3_546_895`, `OutputRate=44_100`, `minPeriod=113`, `maxPeriod=907`

**`effects.go`** — ProTracker effect handlers

- `applyEffectTick0(v, n, r)` — tick-0: set state (porta target, vibrato params, volume, …)
- `applyEffectTickN(v, n, tick)` — tick N: mutate period/volume each tick

## mod/

- `PTModule`, `Pattern`, `Row`, `Note`, `SampleData` — pure structs, no I/O
- `PeriodLookup` — Amiga period → note name table
- `PTModule.PositionAt(dur)` — walks song timeline to find pattern+row at time offset

## loader/

`LoadPTModule(io.ReadSeeker)` — parses binary MOD. Supports 31-sample (M.K., FLT4, …) and 15-sample ORIG variants.

## converter/

`Mod2Wav.Convert(*mod.PTModule) ([]byte, error)` — drives RenderTick loop, applies mid/side stereo separation, encodes RIFF/WAV.

Stereo separation (mid/side):
```
mid  = (L + R) * 0.5
side = (L - R) * (sep/100) * 0.5
outL = mid + side
outR = mid - side
```
sep=0 → mono mix; sep=100 → full Amiga hard pan.

## player/ (module: protracker-player)

`ModPlayer.Init()` — creates oto context (44100 Hz, stereo, int16 LE).  
`ModPlayer.Play(*mod.PTModule, stereoSep)` — wraps ReplayerState in `ModReader` (io.Reader), hands to oto.  
`ModPlayer.Wait()` — blocks until playback ends; also holds `*oto.Player` reference to prevent GC cleanup.

**GC note:** oto v3 uses `runtime.AddCleanup` on its player. If the `*oto.Player` becomes unreachable, GC collects it and the cleanup calls `Close()` → silence. `Wait()` keeps it alive.

## go.work

```
use (
    .        ← protracker-go
    ./player ← protracker-player
)
```

`go work sync` if module graph drifts.
