# Workspace restructure plan

## Goal

2 modules in one Go workspace:
1. `protracker-go` — existing code (PT parsing, loader, WAV converter)
2. `protracker-player` — real-time player using oto, takes `*mod.PTModule`

## Layout

```
protracker-go/           ← module 1 root: "protracker-go" (unchanged)
  go.work                ← NEW: workspace file
  go.mod                 ← unchanged
  main.go
  mod/pt.go
  loader/loader.go
  converter/...

  player/                ← module 2: "protracker-player"
    go.mod               ← requires protracker-go, oto
    replayer.go          ← real-time player, takes *mod.PTModule
```

## go.work

```
go 1.26

use (
    .
    ./player
)
```

## Steps

1. Create `go.work` at repo root
2. Create `player/go.mod` with module `protracker-player`, require `protracker-go`
3. Implement `player/replayer.go` — real-time replayer using oto
4. `go work sync`
