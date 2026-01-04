package serpent_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/adamkeys/serpent"
)

func TestLoad_SingleCall(t *testing.T) {
	program := serpent.Program[int, int]("def run(input): return input + 2")
	exec, err := serpent.Load(program)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	defer exec.Close()

	result, err := exec.Run(1)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	const exp = 3
	if result != exp {
		t.Errorf("expected %d; got %d", exp, result)
	}
}

func TestLoad_MultipleCalls(t *testing.T) {
	program := serpent.Program[int, int]("def run(input): return input * 2")
	exec, err := serpent.Load(program)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	defer exec.Close()

	for i := 0; i < 5; i++ {
		result, err := exec.Run(i)
		if err != nil {
			t.Fatalf("run(%d): %v", i, err)
		}
		if result != i*2 {
			t.Errorf("run(%d): expected %d; got %d", i, i*2, result)
		}
	}
}

func TestLoad_ParallelCalls(t *testing.T) {
	program := serpent.Program[int, int]("def run(input): return input + 1")
	exec, err := serpent.Load(program)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	defer exec.Close()

	const n = 20
	results := make([]int, n)
	errs := make([]error, n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = exec.Run(i)
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Errorf("run(%d): %v", i, errs[i])
		} else if results[i] != i+1 {
			t.Errorf("run(%d): expected %d; got %d", i, i+1, results[i])
		}
	}
}

func TestLoad_StatePersists(t *testing.T) {
	program := serpent.Program[int, int](`
counter = 0
def run(input):
    global counter
    counter += input
    return counter
`)
	exec, err := serpent.Load(program)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	defer exec.Close()

	// State accumulates because executable is pinned to one worker: 1, 1+2=3, 3+3=6
	expected := []int{1, 3, 6}
	for i, exp := range expected {
		result, err := exec.Run(i + 1)
		if err != nil {
			t.Fatalf("run(%d): %v", i+1, err)
		}
		if result != exp {
			t.Errorf("run(%d): expected %d; got %d", i+1, exp, result)
		}
	}
}

func TestLoadWriter_MultipleCalls(t *testing.T) {
	program := serpent.Program[int, serpent.Writer](`
def run(input, writer):
    writer.write(str(input * 2).encode())
`)
	exec, err := serpent.LoadWriter(program)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	defer exec.Close()

	for i := 1; i <= 3; i++ {
		var buf bytes.Buffer
		if err := exec.Run(&buf, i); err != nil {
			t.Fatalf("run(%d): %v", i, err)
		}
		exp := fmt.Sprintf("%d", i*2)
		if s := buf.String(); s != exp {
			t.Errorf("run(%d): expected %q; got %q", i, exp, s)
		}
	}
}

func TestLoadWriter_SingleCall(t *testing.T) {
	program := serpent.Program[string, serpent.Writer](`
def run(input, writer):
    writer.write(input.encode())
`)
	exec, err := serpent.LoadWriter(program)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	defer exec.Close()

	var buf bytes.Buffer
	if err := exec.Run(&buf, "pinned"); err != nil {
		t.Fatalf("run: %v", err)
	}

	const exp = "pinned"
	if s := buf.String(); s != exp {
		t.Errorf("expected %q; got %q", exp, s)
	}
}

func TestRun_Add(t *testing.T) {
	program := serpent.Program[int, int]("def run(input): return input + 2")
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
	program := serpent.Program[struct{ Name string }, string]("def run(input): return input['Name']")
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
	const exp = "\"test\""
	program := serpent.Program[string, string]("def run(input): return input")
	result, err := serpent.Run(program, exp)
	if err != nil {
		t.Fatalf("run result: %v", err)
	}

	if result != exp {
		t.Errorf("unexpected result: %q; got: %q", exp, result)
	}
}

func TestRun_ImportTwice(t *testing.T) {
	program := serpent.Program[int, int]("import os\ndef run(input): return input + 2")
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
	cases := []struct {
		name string
		code string
		exp  string
	}{
		{
			"SyntaxError",
			"def run(input): (",
			"was never closed",
		},
		{
			"NameError",
			"def run(input): return undefined_var",
			"name 'undefined_var' is not defined",
		},
		{
			"ZeroDivisionError",
			"def run(input): return 1 / 0",
			"division by zero",
		},
		{
			"TypeError",
			"def run(input): return 'string' + 1",
			"can only concatenate str",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			program := serpent.Program[string, string](tc.code)
			_, err := serpent.Run(program, "test")
			if !errors.Is(err, serpent.ErrRunFailed) {
				t.Errorf("expected ErrRunFailed; got: %v", err)
			}
			if err == nil || !contains(err.Error(), tc.exp) {
				t.Errorf("expected error containing: %q; got: %v", tc.exp, err)
			}
		})
	}
}

func TestRun_NoRunFunction(t *testing.T) {
	program := serpent.Program[string, string]("x = 1")
	_, err := serpent.Run(program, "test")
	if !errors.Is(err, serpent.ErrRunFailed) {
		t.Errorf("expected error: %v; got: %v", serpent.ErrRunFailed, err)
	}
	if err == nil || !contains(err.Error(), "run() function not defined") {
		t.Errorf("expected error containing 'run() function not defined'; got: %v", err)
	}
}

func TestRun_SlowExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test")
	}

	program := serpent.Program[string, string](`
import time
def run(input):
    time.sleep(1)
    return input
`)
	result, err := serpent.Run(program, "test")
	if err != nil {
		t.Fatalf("run result: %v", err)
	}
	if result != "test" {
		t.Errorf("expected result: %q; got: %q", "test", result)
	}
}

func TestRun_MultiExecution(t *testing.T) {
	program := serpent.Program[*struct{}, int]("def run(input): return 1 + 1")
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

func TestRun_FunctionScope(t *testing.T) {
	program := serpent.Program[*struct{}, int](`import math
def calc():
    return int(math.sqrt(4))
def run(input):
    return calc()
`)
	result, err := serpent.Run(program, nil)
	if err != nil {
		t.Fatalf("run result: %v", err)
	}

	const exp = 2
	if result != exp {
		t.Errorf("unexpected result: %q; got: %q", exp, result)
	}
}

func TestRunWrite_WriteOK(t *testing.T) {
	var buf bytes.Buffer
	program := serpent.Program[*struct{}, serpent.Writer](`
def run(input, writer):
    writer.write(b'OK')
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

func TestMain(m *testing.M) {
	// Test that running without Init panics with PythonNotInitialized. This is considered to
	// be a test case but cannot be in its own test function as the library initialization is global.
	func() {
		defer func() {
			if _, ok := recover().(serpent.PythonNotInitialized); !ok {
				panic("expected PythonNotInitialized panic")
			}
		}()
		program := serpent.Program[int, int]("result = input + 2")
		serpent.Run(program, 1)
	}()

	lib, err := serpent.Lib()
	if err != nil {
		fmt.Fprintf(os.Stderr, "set LIBPYTHON_PATH: %v", err)
		os.Exit(1)
	}
	if err := serpent.Init(lib); err != nil && !errors.Is(err, serpent.ErrAlreadyInitialized) {
		fmt.Fprintf(os.Stderr, "init: %v", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
