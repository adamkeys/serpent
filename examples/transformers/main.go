package main

import (
	"fmt"
	"log"
	"strings"

	_ "embed"

	"github.com/adamkeys/serpent"
)

//go:embed entity.py
var program serpent.Program[string, []string]

func main() {
	lib, err := serpent.Lib()
	if err != nil {
		log.Fatalf("failed to find serpent library: %v", err)
	}
	// transformers package requires a single-worker initialization.
	if err := serpent.InitSingleWorker(lib); err != nil {
		log.Fatalf("failed to initialize serpent: %v", err)
	}
	entities, err := serpent.Run(program, "Apple was founded by Steve Jobs.")
	if err != nil {
		log.Fatalf("run result: %v", err)
	}
	fmt.Printf("found entities: %s\n", strings.Join(entities, ", "))
}
