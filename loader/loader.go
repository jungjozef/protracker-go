package loader

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"protracker-go/mod"
)

const (
	songNameSize     = 20
	sampleHeaderSize = 30
	patternSize      = 1024
	rowsPerPattern   = 64
	channelsPerRow   = 4
	bytesPerNote     = 4
	songPosCount     = 128

	// 31-sample module offsets
	variantTagOffset = int64(1080)
	patternStart31   = int64(1084)

	// 15-sample ORIG module offsets
	patternStartORIG = int64(600)
)

type loader struct {
	r      io.ReadSeeker
	module *mod.PTModule
}

func (l *loader) readAt(offset int64, n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := l.r.Seek(offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek %d: %w", offset, err)
	}
	if _, err := io.ReadFull(l.r, buf); err != nil {
		return nil, fmt.Errorf("read %d bytes at offset %d: %w", n, offset, err)
	}
	return buf, nil
}

func (l *loader) readSongName() error {
	buf, err := l.readAt(0, songNameSize)
	if err != nil {
		return err
	}
	l.module.SongName = toCleanString(buf)
	return nil
}

// detectVariant reads the 4-byte tag at offset 1080.
// For 31-sample modules this is a printable ASCII tag ("M.K.", "FLT4", etc.).
// For ORIG (15-sample) modules this offset falls inside pattern data and is
// unlikely to be printable ASCII — that's how we detect them.
func (l *loader) detectVariant() error {
	buf, err := l.readAt(variantTagOffset, 4)
	if err != nil {
		return err
	}
	for _, b := range buf {
		if b < 32 || b > 126 {
			l.module.NumberOfSamples = 15
			l.module.Variant = "ORIG"
			return nil
		}
	}
	l.module.NumberOfSamples = 31
	l.module.Variant = string(buf)
	return nil
}

// readSongInfo reads song length and the pattern order table.
// Both are located immediately after the sample headers, so the offset
// depends on NumberOfSamples (set by detectVariant).
func (l *loader) readSongInfo() error {
	sampleHeadersEnd := int64(songNameSize + int(l.module.NumberOfSamples)*sampleHeaderSize)

	buf, err := l.readAt(sampleHeadersEnd, 1)
	if err != nil {
		return fmt.Errorf("song length: %w", err)
	}
	l.module.SongLength = buf[0]

	// +1 skips the "restart" byte (NoiseTracker/Soundtracker legacy field)
	posBuf, err := l.readAt(sampleHeadersEnd+2, songPosCount)
	if err != nil {
		return fmt.Errorf("song positions: %w", err)
	}
	l.module.SongPositions = posBuf

	// Only scan valid positions [0:SongLength]; bytes beyond are undefined.
	var maxPattern uint8
	for i := 0; i < int(l.module.SongLength); i++ {
		if posBuf[i] > maxPattern {
			maxPattern = posBuf[i]
		}
	}
	l.module.NumberOfPatterns = maxPattern + 1
	return nil
}

func (l *loader) readSamples() error {
	l.module.SampleData = make([]mod.SampleData, l.module.NumberOfSamples)
	for i := 0; i < int(l.module.NumberOfSamples); i++ {
		offset := int64(songNameSize + i*sampleHeaderSize)
		buf, err := l.readAt(offset, sampleHeaderSize)
		if err != nil {
			return fmt.Errorf("sample %d: %w", i, err)
		}
		s := &l.module.SampleData[i]
		s.Name = toCleanString(buf[0:22])
		s.Length = binary.BigEndian.Uint16(buf[22:24]) * 2
		s.FineTune = buf[24] & 0x0F // low nibble only; sign-extend to int8 for playback
		s.Volume = buf[25]
		s.RepeatStart = binary.BigEndian.Uint16(buf[26:28]) * 2
		s.RepeatLength = binary.BigEndian.Uint16(buf[28:30]) * 2
	}
	return nil
}

func (l *loader) patternBase() int64 {
	if l.module.Variant == "ORIG" {
		return patternStartORIG
	}
	return patternStart31
}

func (l *loader) readPatterns() error {
	base := l.patternBase()
	l.module.Patterns = make([]mod.Pattern, l.module.NumberOfPatterns)
	buf := make([]byte, patternSize)
	for i := 0; i < int(l.module.NumberOfPatterns); i++ {
		if _, err := l.r.Seek(base+int64(i)*patternSize, io.SeekStart); err != nil {
			return fmt.Errorf("pattern %d seek: %w", i, err)
		}
		if _, err := io.ReadFull(l.r, buf); err != nil {
			return fmt.Errorf("pattern %d read: %w", i, err)
		}
		for row := 0; row < rowsPerPattern; row++ {
			for ch := 0; ch < channelsPerRow; ch++ {
				off := row*channelsPerRow*bytesPerNote + ch*bytesPerNote
				b := buf[off : off+bytesPerNote]
				l.module.Patterns[i].Data[row][ch] = mod.Note{
					Value:         (uint16(b[0]&0x0F) << 8) | uint16(b[1]),
					SampleNumber:  b[0]&0xF0 | (b[2]&0xF0)>>4,
					EffectCommand: b[2] & 0x0F,
					EffectData:    b[3],
					Channel:       uint8(ch),
				}
			}
		}
	}
	return nil
}

func (l *loader) readSampleData() error {
	// Sample audio follows all pattern data; base offset depends on variant.
	base := l.patternBase() + int64(l.module.NumberOfPatterns)*patternSize
	var offset int64
	for i := 0; i < int(l.module.NumberOfSamples); i++ {
		s := &l.module.SampleData[i]
		if s.Length == 0 {
			continue
		}
		s.Data = make([]byte, s.Length)
		if _, err := l.r.Seek(base+offset, io.SeekStart); err != nil {
			return fmt.Errorf("sample data %d seek: %w", i, err)
		}
		if _, err := io.ReadFull(l.r, s.Data); err != nil {
			return fmt.Errorf("sample data %d read: %w", i, err)
		}
		offset += int64(s.Length)
	}
	return nil
}

// toCleanString replaces non-printable bytes with spaces on a copy of src.
func toCleanString(src []byte) string {
	b := make([]byte, len(src))
	copy(b, src)
	for i, c := range b {
		if c < 32 || c > 126 {
			b[i] = 32
		}
	}
	return strings.TrimSpace(string(b))
}

// formatNote formats a single note cell as "NOTE SM EFF" (10 chars wide).
// NOTE = pitch (3), SM = sample number (2 hex), EFF = effect cmd+data (3 hex).
func formatNote(n mod.Note) string {
	pitch := n.NoteToString()
	if pitch == "" {
		pitch = "???"
	}
	return fmt.Sprintf("%s %02X %1X%02X", pitch, n.SampleNumber, n.EffectCommand, n.EffectData)
}

// FormatModule returns a human-readable representation of a parsed PTModule.
// Layout: metadata → sample table → full pattern data.
func FormatModule(m *mod.PTModule) string {
	var sb strings.Builder

	// Metadata
	fmt.Fprintf(&sb, "=== %s ===\n", m.SongName)
	fmt.Fprintf(&sb, "Variant: %-6s  Samples: %d  Song length: %d patterns\n\n",
		m.Variant, m.NumberOfSamples, m.SongLength)

	// Pattern order
	fmt.Fprintf(&sb, "--- Pattern Order ---\n")
	for i := 0; i < int(m.SongLength); i++ {
		fmt.Fprintf(&sb, "%02X ", m.SongPositions[i])
	}
	fmt.Fprintf(&sb, "\n\n")

	// Sample table
	fmt.Fprintf(&sb, "--- Samples ---\n")
	fmt.Fprintf(&sb, " #  %-22s  %6s  %3s  %4s  %8s  %6s\n",
		"Name", "Length", "Vol", "Fine", "RepStart", "RepLen")
	for i, s := range m.SampleData {
		if s.Length == 0 && s.Name == "" {
			continue
		}
		fmt.Fprintf(&sb, "%2d  %-22s  %6d  %3d  %4d  %8d  %6d\n",
			i+1, s.Name, s.Length, s.Volume, s.FineTune, s.RepeatStart, s.RepeatLength)
	}
	fmt.Fprintf(&sb, "\n")

	// Pattern data
	fmt.Fprintf(&sb, "--- Patterns ---\n")
	for pi, p := range m.Patterns {
		fmt.Fprintf(&sb, "\nPattern %02X:\n", pi)
		fmt.Fprintf(&sb, "Row | %-10s  %-10s  %-10s  %-10s\n", "Ch 1", "Ch 2", "Ch 3", "Ch 4")
		for row := 0; row < rowsPerPattern; row++ {
			fmt.Fprintf(&sb, " %02X |", row)
			for ch := 0; ch < channelsPerRow; ch++ {
				n := p.Data[row][ch]
				fmt.Fprintf(&sb, " %s ", formatNote(n))
			}
			fmt.Fprintf(&sb, "\n")
		}
	}

	return sb.String()
}

// LoadPTModule parses a ProTracker MOD file from any io.ReadSeeker.
// Supports both 31-sample (M.K., FLT4, etc.) and 15-sample ORIG variants.
func LoadPTModule(r io.ReadSeeker) (*mod.PTModule, error) {
	if r == nil {
		return nil, fmt.Errorf("reader is nil")
	}
	ld := &loader{r: r, module: &mod.PTModule{}}

	steps := []struct {
		name string
		fn   func() error
	}{
		{"song name", ld.readSongName},
		{"variant detection", ld.detectVariant},
		{"song info", ld.readSongInfo},
		{"samples", ld.readSamples},
		{"patterns", ld.readPatterns},
		{"sample data", ld.readSampleData},
	}

	for _, s := range steps {
		if err := s.fn(); err != nil {
			return nil, fmt.Errorf("%s: %w", s.name, err)
		}
	}

	return ld.module, nil
}
