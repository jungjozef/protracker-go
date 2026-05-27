package main

import (
	"flag"
	"fmt"
	"os"

	"protracker-go/converter"
	"protracker-go/loader"

	"protracker-player"
)

func main() {
	inputFilename := flag.String("input", "", "Input MOD filename")
	outputFilename := flag.String("output", "", "Output WAV filename (convert mode only; default: input + .wav)")
	stereoSep := flag.Int("stereo-sep", 30, "Stereo separation 0–100 (0=mono mix, 100=full Amiga hard pan)")
	mode := flag.String("mode", "play", "Mode: play or convert")
	flag.Parse()

	if *inputFilename == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	f, err := os.Open(*inputFilename)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	m, err := loader.LoadPTModule(f)
	if err != nil {
		panic(err)
	}

	switch *mode {
	case "play":
		fmt.Printf("Playing: %s  stereo-sep=%d\n", *inputFilename, *stereoSep)
		rp := player.ModPlayer{}
		rp.Init()
		rp.Play(m, *stereoSep)
		rp.Wait()

	case "convert":
		if *outputFilename == "" {
			*outputFilename = *inputFilename + ".wav"
		}
		fmt.Printf("Converting: %s → %s  stereo-sep=%d\n", *inputFilename, *outputFilename, *stereoSep)
		c := converter.NewMod2Wav(converter.Stereo, *stereoSep)
		wav, err := c.Convert(m)
		if err != nil {
			panic(err)
		}
		outf, err := os.OpenFile(*outputFilename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			panic(err)
		}
		defer outf.Close()
		if _, err := outf.Write(wav); err != nil {
			panic(err)
		}
		fmt.Printf("Written %d bytes to %s\n", len(wav), *outputFilename)

	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q — use play or convert\n", *mode)
		os.Exit(1)
	}
}
