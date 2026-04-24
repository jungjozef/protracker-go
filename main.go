package main

import (
	"fmt"
	"os"
)

func main() {
	f, err := os.Open("le_baloche_a_dede.mod")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	m, err := LoadPTModule(f)
	if err != nil {
		panic(err)
	}

	fmt.Println(m)
}
