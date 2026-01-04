package main

import (
	"fmt"
	"log"

	"github.com/adamkeys/serpent"
)

func main() {
	lib, err := serpent.Lib()
	if err != nil {
		log.Fatalf("failed to find serpent library: %v", err)
	}
	if err := serpent.Init(lib); err != nil {
		log.Fatalf("failed to initialize serpent: %v", err)
	}

	program := serpent.Program[int, int]("def run(input): return input")
	fmt.Println(serpent.Run(program, 42))
}
