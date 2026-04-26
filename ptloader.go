package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"protracker-go/mod"
)

type loader struct {
	file   *os.File
	module *mod.PTModule
	buf    []byte
}

func (l *loader) songName() {
	println("Reading song name")
	l.buf = make([]byte, 20)
	l.file.Seek(0, io.SeekStart)
	_, _ = io.ReadFull(l.file, l.buf)
	for i := 0; i < 20; i++ {
		// set every 0 to ascii space
		if l.buf[i] == 0 {
			l.buf[i] = 32
		}
	}

	l.module.SongName = strings.TrimSpace(string(l.buf))
}

func (l *loader) nrOfPatterns() {
	println("determining number of samples")
	l.file.Seek(1080, io.SeekStart)
	l.buf = make([]byte, 4)
	_, _ = io.ReadFull(l.file, l.buf)
	l.module.NumberOfSamples = 31
	for i := 0; i < 4; i++ {
		if l.buf[i] < 32 || l.buf[i] > 126 {
			l.module.NumberOfSamples = 15
			l.module.Variant = "ORIG"
			break
		}
	}
	if l.module.Variant != "ORIG" {
		l.module.Variant = string(l.buf)
	}

	println("determining songlen")
	l.file.Seek(int64(20+l.module.NumberOfSamples*30), io.SeekStart)
	l.buf = make([]byte, 1)
	io.ReadFull(l.file, l.buf)
	l.module.SongLength = l.buf[0]
	println("determining song positions")
	l.file.Seek(int64(22+l.module.NumberOfSamples*30), io.SeekStart)
	l.module.SongPositions = make([]byte, 128)
	io.ReadFull(l.file, l.module.SongPositions)
	fmt.Printf("songlen: %d\n", l.module.SongLength)
	var nrOfPatterns uint8 = 0
	for i := 0; i < int(l.module.SongLength); i++ {
		if l.module.SongPositions[i] > nrOfPatterns {
			nrOfPatterns = l.module.SongPositions[i]
		}
		fmt.Printf("pos: %d, pattern: %d\n", i, l.module.SongPositions[i])
	}
	l.module.NumberOfPatterns = nrOfPatterns + 1
	fmt.Printf("nr of patterns: %d\n", l.module.NumberOfPatterns)

}

func (l *loader) readSamples() {
	sampleInfoLength := 30
	l.buf = make([]byte, sampleInfoLength)
	l.module.SampleData = make([]mod.SampleData, l.module.NumberOfSamples)
	for i := 0; i < int(l.module.NumberOfSamples); i++ {
		l.file.Seek(20+int64(i*sampleInfoLength), io.SeekStart)
		io.ReadFull(l.file, l.buf)
		l.module.SampleData[i].Name = l.toString(l.buf[0:22])
		l.module.SampleData[i].Length = binary.BigEndian.Uint16(l.buf[22:24]) * 2
		l.module.SampleData[i].FineTune = l.buf[24]
		l.module.SampleData[i].Volume = l.buf[25]
		l.module.SampleData[i].RepeatStart = binary.BigEndian.Uint16(l.buf[26:28]) * 2
		l.module.SampleData[i].RepeatLength = binary.BigEndian.Uint16(l.buf[28:30]) * 2
	}
}

func (l *loader) readPatterns() {
	l.buf = make([]byte, 1024)
	patternStart := 1084
	if l.module.Variant == "ORIG" {
		patternStart = 600
	}
	l.module.Patterns = make([]mod.Pattern, l.module.NumberOfPatterns)
	for i := 0; i < int(l.module.NumberOfPatterns); i++ {
		l.file.Seek(int64(patternStart+(i*1024)), io.SeekStart)
		io.ReadFull(l.file, l.buf)
		// buf now holds a pattern
		// go row by row
		for j := 0; j < 64; j += 1 {
			fmt.Println("reading row:", j)
			notes := l.buf[j*16 : (j*16)+16]
			for k := 0; k < 4; k += 1 {
				ch := notes[4*k : (4*k)+4]
				note := mod.Note{
					Value:         (uint16(ch[0]&0x0F) << 8) | uint16(ch[1]),
					SampleNumber:  ch[0]&0xF0 | (ch[2]&0xF0)>>4,
					EffectCommand: ch[2] & 0x0F,
					EffectData:    ch[3],
					Channel:       uint8(k),
				}
				l.module.Patterns[i].Data[j][k] = note
				fmt.Printf("%s\n", note.String())
			}
		}
	}
}

func (l *loader) readSampleData() {
	var offset int64 = 0
	sampleDataStart := 1084 + (int64(l.module.NumberOfPatterns) * 1024)
	for i := 0; i < int(l.module.NumberOfSamples); i++ {
		if l.module.SampleData[i].Length == 0 {
			continue
		}
		l.file.Seek(offset+sampleDataStart, io.SeekStart)
		l.module.SampleData[i].Data = make([]byte, l.module.SampleData[i].Length)
		io.ReadFull(l.file, l.module.SampleData[i].Data)
		offset += int64(l.module.SampleData[i].Length)
	}
}

func (l *loader) toString(b []byte) string {
	for i := 0; i < len(b); i++ {
		if b[i] < 32 || b[i] > 126 {
			b[i] = 32
		}
	}
	return strings.TrimSpace(string(b))
}

func LoadPTModule(file *os.File) (*mod.PTModule, error) {
	if file == nil {
		return nil, errors.New("file is nil")
	}
	ld := &loader{
		file:   file,
		module: &mod.PTModule{},
	}
	ld.songName()
	ld.nrOfPatterns()
	ld.readSamples()
	ld.readPatterns()
	ld.readSampleData()

	return ld.module, nil
}
