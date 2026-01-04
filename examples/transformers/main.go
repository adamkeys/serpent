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

	exec, err := serpent.Load(program)
	if err != nil {
		log.Fatalf("load program: %v", err)
	}
	defer exec.Close()

	inputs := []string{
		"Apple was founded by Steve Jobs in Cupertino, California.",
		"Google was founded by Larry Page and Sergey Brin while they were Ph.D. students at Stanford University.",
		"Microsoft was founded by Bill Gates and Paul Allen on April 4, 1975, in Albuquerque, New Mexico.",
	}
	for _, input := range inputs {
		entities, err := exec.Run(input)
		if err != nil {
			log.Fatalf("run result: %v", err)
		}
		fmt.Printf("found entities: %s\n", strings.Join(entities, ", "))
	}
}
