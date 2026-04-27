package main

import (
	"fmt"
	"os"

	"protracker-go/converter"
)

func main() {
	f, err := os.Open("h0ffman_-_drop_the_panic.mod")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	m, err := LoadPTModule2(f)
	if err != nil {
		panic(err)
	}

	fmt.Println(FormatModule(m))

	c := converter.NewMod2Wav(converter.Stereo, 30)
	wav, err := c.Convert(m)
	if err != nil {
		panic(err)

	}
	outf, err := os.OpenFile("output.wav", os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		panic(err)
	}
	defer outf.Close()
	outf.Write(wav)
}
