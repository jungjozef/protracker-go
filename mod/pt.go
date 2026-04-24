package mod

import (
	"errors"
	"fmt"
	"strings"
)

type PTModule struct {
	SongName         string
	Variant          string
	NumberOfSamples  uint8
	SampleData       []SampleData
	SongLength       uint8
	SongPositions    []byte
	NumberOfPatterns uint8
	Patterns         []Pattern
}

type Pattern struct {
	Data [64]Row
}

var periodLookup = []uint16{
	//C   C#    D     D#    E     F     F#    G     G#    A     A#   B
	1712, 1616, 1524, 1440, 1356, 1280, 1208, 1140, 1076, 1016, 960, 907, // 0ctave 0
	856, 808, 762, 720, 678, 640, 604, 570, 538, 508, 480, 453, // 0ctave 1
	428, 404, 381, 360, 339, 320, 302, 285, 269, 254, 240, 226, // 0ctave 2
	214, 202, 190, 180, 170, 160, 151, 143, 135, 127, 120, 113, // 0ctave 3
	107, 101, 95, 90, 85, 80, 75, 71, 67, 63, 60, 56, // 0ctave 4
}

var notes = []string{
	"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B",
}

type Row [4]Note
type Note struct {
	Value         uint16
	SampleNumber  uint8
	EffectCommand uint8
	EffectData    uint8
	Channel       uint8
}

func (n *Note) String() string {
	note := n.NoteToString()
	return fmt.Sprintf("%s %02x %02x %02x", note, n.SampleNumber, n.EffectCommand, n.EffectData)
}

func (n *Note) NoteToString() string {
	if n.Value == 0 {
		return "---"
	}
	// First find the octave
	octave := 0
	for i := 0; i < len(periodLookup); i += 12 {
		if n.Value > periodLookup[i] {
			break
		} else {
			octave++
		}
	}
	octave--
	// Now find the note within that octave
	offset := octave * 12
	position := 0
	for i := offset; i < offset+12; i++ {
		if periodLookup[i] == n.Value {
			position = i
			break
		}
	}

	if position == -1 {
		println("unable to find note for period")
		return ""
	}

	note, err := periodToString(position, octave)
	if err != nil {
		println(err)
		return ""
	}

	return note
}

func periodToString(periodPos int, octave int) (string, error) {
	if (periodPos < 0) || (periodPos > len(periodLookup)) {
		return "", errors.New("invalid period to convert to string")
	}

	// Get the note pitch
	noteLookupPos := periodPos % 12
	pitch := notes[noteLookupPos]
	if strings.HasSuffix(pitch, "#") {
		return fmt.Sprintf("%s%d", pitch, octave), nil
	}

	return fmt.Sprintf("%s-%d", pitch, octave), nil
}

type SampleData struct {
	Name         string
	Length       uint16
	FineTune     byte
	Volume       byte
	RepeatStart  uint16
	RepeatLength uint16
	Data         []byte
}

func (sd *SampleData) String() string {
	return fmt.Sprintf("Name: %s, Length: %d, FineTune: %b, Volume: %d, RepeatStart: %d, RepeatLength: %d", sd.Name, sd.Length, sd.FineTune, sd.Volume, sd.RepeatStart, sd.RepeatLength)
}

func (pt *PTModule) String() string {
	sd := "["
	for i := 0; i < len(pt.SampleData); i++ {
		sd += fmt.Sprintf("{ Sample %d: %s }, ", i, pt.SampleData[i].String())
	}
	sd = strings.TrimRight(sd, ", ") + "]"
	return fmt.Sprintf("Name: %s, Variant: %s, NumberOfSamples: %d, SampleData: %s", pt.SongName, pt.Variant, pt.NumberOfSamples, sd)
}
