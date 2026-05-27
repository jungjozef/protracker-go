# protracker-go

Amiga ProTracker MOD player and converter in Go.

## Features

- Parses 31-sample and 15-sample (ORIG) MOD files
- Real-time playback via system audio (oto v3)
- Converts to 44100 Hz, 16-bit PCM WAV
- Stereo output with configurable mid/side separation
- Full ProTracker effect set (0xx–Fxx) — see [docs/effects.md](docs/effects.md)
- Fine-tune support (signed 4-bit per sample)
- BLEP anti-aliasing (band-limited step, removes loop/end-of-sample click artifacts)
- Optional Amiga hardware low-pass filter (~4.4 kHz, toggle with `-filter`)

## Usage

```sh
# Play
go run . -input song.mod -stereo-sep 30

# Play with Amiga filter
go run . -input song.mod -stereo-sep 30 -filter

# Convert to WAV
go run . -mode convert -input song.mod -output song.wav -stereo-sep 30
```

| Flag | Default | Description |
|------|---------|-------------|
| `-mode` | `play` | `play` or `convert` |
| `-input` | — | Input `.mod` file |
| `-output` | input + `.wav` | Output file (convert mode) |
| `-stereo-sep` | `30` | Stereo separation 0–100 (0=mono mix, 100=hard pan) |
| `-filter` | `false` | Amiga hardware low-pass filter (~4.4 kHz cutoff) |

## Structure

```
mod/        — PTModule data structures
loader/     — MOD file parser
engine/     — Core render engine (ReplayerState, RenderTick, effects)
converter/  — WAV encoder (uses engine)
player/     — Real-time audio player module (uses engine + oto v3)
```

See [docs/architecture.md](docs/architecture.md) for details.

## Build & Test

```sh
go build ./...
go test ./...
```

Requires Go 1.26+.

## Known Limitations

- No BLEP anti-aliasing (click artifacts at sample loop/retrigger points)
