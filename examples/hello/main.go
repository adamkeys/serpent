package main

import (
	"log"
	"os"
	"sync"

	_ "embed"

	"github.com/adamkeys/serpent"
)

//go:embed hello.py
var program serpent.Program[int, serpent.Writer]

func main() {
	lib, err := serpent.Lib()
	if err != nil {
		log.Fatalf("failed to find serpent library: %v", err)
	}
	if err := serpent.Init(lib); err != nil {
		log.Fatalf("failed to initialize serpent: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			defer wg.Done()
			if err := serpent.RunWrite(os.Stdout, program, i); err != nil {
				log.Fatalf("run write: %v", err)
			}
		}(i)
	}
	wg.Wait()
}
