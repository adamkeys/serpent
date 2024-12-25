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

func TestRun_Add(t *testing.T) {
	initPython(t)

	program := serpent.Program[int, int]("result = input + 2")
	result, err := serpent.Run(program, 1)
	if err != nil {
		t.Fatalf("run result: %v", err)
	}

	const exp = 3
	if result != exp {
		t.Errorf("unexpected result: %d; got: %d", exp, result)
	}
}

func TestRun_Struct(t *testing.T) {
	initPython(t)

	program := serpent.Program[struct{ Name string }, string]("result = input['Name']")
	result, err := serpent.Run(program, struct{ Name string }{Name: "test"})
	if err != nil {
		t.Fatalf("run result: %v", err)
	}

	const exp = "test"
	if result != exp {
		t.Errorf("unexpected result: %q; got: %q", exp, result)
	}
}

func TestRun_EscapedString(t *testing.T) {
	initPython(t)

	const exp = "\"test\""
	program := serpent.Program[string, string]("result = input")
	result, err := serpent.Run(program, exp)
	if err != nil {
		t.Fatalf("run result: %v", err)
	}

	if result != exp {
		t.Errorf("unexpected result: %q; got: %q", exp, result)
	}
}

func TestRun_ImportTwice(t *testing.T) {
	initPython(t)

	program := serpent.Program[int, int]("import os; result = input + 2")
	_, err := serpent.Run(program, 1)
	if err != nil {
		t.Fatalf("run result: %v", err)
	}
	_, err = serpent.Run(program, 1)
	if err != nil {
		t.Fatalf("run result: %v", err)
	}
}

func TestRun_InvalidProgram(t *testing.T) {
	initPython(t)

	program := serpent.Program[string, string]("(")
	_, err := serpent.Run(program, "test")
	if !errors.Is(err, serpent.ErrRunFailed) {
		t.Errorf("expected error: %v; got: %v", serpent.ErrRunFailed, err)
	}
}

func TestRun_NoResult(t *testing.T) {
	initPython(t)

	program := serpent.Program[string, string]("input")
	_, err := serpent.Run(program, "test")
	if !errors.Is(err, serpent.ErrNoResult) {
		t.Errorf("expected error: %v; got: %v", serpent.ErrNoResult, err)
	}
}

func TestRun_SlowExecution(t *testing.T) {
	initPython(t)

	program := serpent.Program[string, string]("import time; time.sleep(1); result = input")
	result, err := serpent.Run(program, "test")
	if err != nil {
		t.Fatalf("run result: %v", err)
	}
	if result != "test" {
		t.Errorf("expected result: %q; got: %q", "test", result)
	}
}

func TestRun_MultiExecution(t *testing.T) {
	initPython(t)

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
}

func TestRunWrite_WriteOK(t *testing.T) {
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
	if err := serpent.Init(lib); err != nil && !errors.Is(err, serpent.ErrAlreadyInitialized) {
		t.Fatalf("init: %v", err)
	}
}
