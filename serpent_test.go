package serpent_test

import (
	"bytes"
	"errors"
	"sync"
	"testing"

	"github.com/adamkeys/serpent"
)

func TestInitRequired(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected a panic")
		}
	}()

	program := serpent.Program[int, int]("result = input + 2")
	serpent.Run(program, 1)
}

func TestRun(t *testing.T) {
	initPython(t)

	t.Run("Add", func(t *testing.T) {
		program := serpent.Program[int, int]("result = input + 2")
		result, err := serpent.Run(program, 1)
		if err != nil {
			t.Fatalf("run result: %v", err)
		}

		const exp = 3
		if result != exp {
			t.Errorf("unexpected result: %d; got: %d", exp, result)
		}
	})

	t.Run("Struct", func(t *testing.T) {
		program := serpent.Program[struct{ Name string }, string]("result = input['Name']")
		result, err := serpent.Run(program, struct{ Name string }{Name: "test"})
		if err != nil {
			t.Fatalf("run result: %v", err)
		}

		const exp = "test"
		if result != exp {
			t.Errorf("unexpected result: %q; got: %q", exp, result)
		}
	})

	t.Run("InvalidProgram", func(t *testing.T) {
		program := serpent.Program[string, string]("(")
		_, err := serpent.Run(program, "test")
		if !errors.Is(err, serpent.ErrRunFailed) {
			t.Errorf("expected error: %v; got: %v", serpent.ErrRunFailed, err)
		}
	})

	t.Run("NoResult", func(t *testing.T) {
		program := serpent.Program[string, string]("input")
		_, err := serpent.Run(program, "test")
		if !errors.Is(err, serpent.ErrNoResult) {
			t.Errorf("expected error: %v; got: %v", serpent.ErrNoResult, err)
		}
	})

	t.Run("MultiExecution", func(t *testing.T) {
		program := serpent.Program[*struct{}, int]("result = 1 + 1")
		results := make([]int, 10)
		errCh := make(chan error, len(results))
		var wg sync.WaitGroup
		wg.Add(len(results))
		for i := 0; i < len(results); i++ {
			go func(i int) {
				defer wg.Done()
				result, err := serpent.Run(program, nil)
				if err != nil {
					errCh <- err
					return
				}
				results[i] = result
			}(i)
		}
		wg.Wait()
		close(errCh)

		for err := range errCh {
			if err != nil {
				t.Fatalf("run result: %v", err)
			}
		}

		for i, result := range results {
			if result != 2 {
				t.Errorf("unexpected result: 2; got: %d (index: %d)", result, i)
			}
		}
	})
}

func TestRunWrite(t *testing.T) {
	initPython(t)

	var buf bytes.Buffer
	program := serpent.Program[*struct{}, serpent.Writer](`
import os
os.write(fd, b'OK')
`)
	err := serpent.RunWrite(&buf, program, nil)
	if err != nil {
		t.Fatalf("run result: %v", err)
	}

	const exp = "OK"
	if s := buf.String(); s != exp {
		t.Errorf("unexpected result: %q; got: %q", exp, s)
	}
}

func initPython(t testing.TB) {
	t.Helper()

	lib, err := serpent.Lib()
	if err != nil {
		t.Fatalf("set LIBPYTHON_PATH: %v", err)
	}
	if err := serpent.Init(lib); err != nil {
		t.Fatalf("init: %v", err)
	}
}
