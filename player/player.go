package player

import (
	"encoding/binary"
	"io"
	"time"

	"protracker-go/engine"
	"protracker-go/mod"

	"github.com/ebitengine/oto/v3"
)

type ModPlayer struct {
	ctx    *oto.Context
	player *oto.Player
}

func (m *ModPlayer) Init() {
	op := &oto.NewContextOptions{
		SampleRate:   44100,
		ChannelCount: 2,
		Format:       oto.FormatSignedInt16LE,
	}
	ctx, ready, err := oto.NewContext(op)
	if err != nil {
		panic(err)
	}
	<-ready
	m.ctx = ctx
}

// ModReader wraps ReplayerState as an io.Reader for oto.
// oto calls Read() on its own goroutine; we render ticks on demand.
type ModReader struct {
	replayerState *engine.ReplayerState
	buf           []float64 // rendered samples not yet consumed (interleaved L,R float64)
	stereoSep     int       // 0 = mono/full-mix, 100 = full Amiga hard panning
}

func NewModReader(m *mod.PTModule, stereoSep int) *ModReader {
	return &ModReader{
		replayerState: engine.NewReplayerState(m),
		stereoSep:     stereoSep,
	}
}

// Read implements io.Reader.
// p holds int16 LE stereo samples: [L0_lo, L0_hi, R0_lo, R0_hi, L1_lo, ...]
// Each stereo frame = 4 bytes.
func (m *ModReader) Read(p []byte) (int, error) {
	need := len(p) / 4 // stereo int16: 4 bytes per frame
	if need == 0 {
		return 0, nil
	}

	// Render ticks until buffer holds enough samples (or song ends).
	for len(m.buf) < need*2 {
		if m.replayerState.Done {
			break
		}
		m.buf = append(m.buf, engine.RenderTick(m.replayerState)...)
	}

	// How many complete stereo frames are ready?
	available := len(m.buf) / 2
	if available == 0 {
		return 0, io.EOF
	}
	if need > available {
		need = available
	}

	// Convert float64 → int16 LE with stereo separation (mid/side formula).
	for i := 0; i < need; i++ {
		l, r := applyMix(m.buf[i*2], m.buf[i*2+1], m.stereoSep)
		binary.LittleEndian.PutUint16(p[i*4:], uint16(f64ToI16(l)))
		binary.LittleEndian.PutUint16(p[i*4+2:], uint16(f64ToI16(r)))
	}
	m.buf = m.buf[need*2:]
	return need * 4, nil
}

// applyMix normalises 4-voice sum and applies mid/side stereo separation.
//
//	sep=0   → pure mono (mid only, fed to both channels)
//	sep=100 → full Amiga hard panning (channels 0,2 left; 1,3 right)
func applyMix(left, right float64, sep int) (float64, float64) {
	// Amiga panning: 2 voices max per side (ch 0,2→L; ch 1,3→R).
	// Divide by 2, not 4, so full-load side hits 1.0 not 0.5.
	left /= 2.0
	right /= 2.0
	mid := (left + right) * 0.5
	side := (left - right) * (float64(sep) / 100.0) * 0.5
	return mid + side, mid - side
}

func (m *ModPlayer) Play(mo *mod.PTModule, stereoSep int) {
	m.player = m.ctx.NewPlayer(NewModReader(mo, stereoSep))
	m.player.Play()
}

// Wait blocks until the player finishes.
// Keeping m.player referenced here prevents the GC from collecting
// the oto player object (and its cleanup) while the song is still playing.
func (m *ModPlayer) Wait() {
	for m.player.IsPlaying() {
		time.Sleep(100 * time.Millisecond)
	}
}

// f64ToI16 clamps and scales a float64 sample to int16 range.
func f64ToI16(s float64) int16 {
	if s > 1.0 {
		s = 1.0
	} else if s < -1.0 {
		s = -1.0
	}
	return int16(s * 32767)
}
