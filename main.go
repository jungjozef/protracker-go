package main

import (
	"flag"
	"fmt"
	"os"

	"protracker-go/converter"
	"protracker-go/loader"
)

func main() {
	inputFilename := flag.String("input", "", "Input filename")
	outputFilename := flag.String("output", "", "Output filename")
	stereoSep := flag.Int("stereo-sep", 30, "Stereo separator")
	flag.Parse()
	if *inputFilename == "" || *outputFilename == "" {
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

	fmt.Println(loader.FormatModule(m))

	c := converter.NewMod2Wav(converter.Stereo, *stereoSep)
	wav, err := c.Convert(m)
	if err != nil {
		panic(err)

	}
	outf, err := os.OpenFile(*outputFilename, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		panic(err)
	}
	defer outf.Close()
	outf.Write(wav)
}
