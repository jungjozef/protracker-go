# protracker-go

Amiga ProTracker MOD file parser and renderer in Go. Converts `.mod` files to WAV.

## Features

- Parses 31-sample and 15-sample (ORIG) MOD files
- Renders to 44100 Hz, 16-bit PCM WAV
- Mono or stereo output with configurable stereo separation
- Effects: Fxx, Cxx, Axx, Bxx, Dxx, 1xx, 2xx, 3xx, 4xx, 0xx, 9xx, Exx

## Usage

```sh
go run . -input song.mod -output song.wav
go run . -input song.mod -output song.wav -stereo-sep 70
```

| Flag | Default | Description |
|------|---------|-------------|
| `-input` | — | Input `.mod` file |
| `-output` | — | Output `.wav` file |
| `-stereo-sep` | 30 | Stereo separation 0–100 (0=mono mix, 100=hard pan) |

## Structure

```
mod/        — PTModule data structures
loader/     — MOD file parser
converter/  — Replayer + WAV encoder
replayer/   — Real-time replayer (WIP)
```

## Build & Test

```sh
go build ./...
go test ./...
```

Requires Go 1.24+.

## Known Limitations

- No BLEP anti-aliasing (some aliasing on sharp note transitions)
- No Amiga low-pass filter emulation