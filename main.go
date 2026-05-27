package main

import (
	"flag"
	"fmt"
	"os"

	"protracker-go/loader"
	"replayer"
)

func main() {
	inputFilename := flag.String("input", "", "Input filename")
	outputFilename := flag.String("output", "", "Output filename")
	stereoSep := flag.Int("stereo-sep", 30, "Stereo separator")
	flag.Parse()
	if *inputFilename == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}
	if *outputFilename == "" {
		*outputFilename = *inputFilename + ".wav"
	}
	println(fmt.Sprintf("Input filename: %s, output filename: %s, stereo sep %d", *inputFilename, *outputFilename, *stereoSep))
	f, err := os.Open(*inputFilename)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	m, err := loader.LoadPTModule(f)
	if err != nil {
		panic(err)
	}

	// fmt.Println(loader.FormatModule(m))
	//
	//c := converter.NewMod2Wav(converter.Stereo, *stereoSep)
	//_, err = c.Convert(m)
	//if err != nil {
	//	panic(err)
	//
	//}
	//outf, err := os.OpenFile(*outputFilename, os.O_WRONLY|os.O_CREATE, 0666)
	//if err != nil {
	//	panic(err)
	//}
	//defer outf.Close()
	//outf.Write(wav)

	rp := replayer.ModPlayer{}
	rp.Init()
	rp.Play(m, *stereoSep)
	rp.Wait() // blocks until song ends; also keeps player alive (prevents GC)
}
