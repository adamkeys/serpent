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

func TestRun_Add(t *testing.T) {
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
	cases := []struct {
		name string
		code string
		exp  string
	}{
		{
			"SyntaxError",
			"(",
			"was never closed",
		},
		{
			"NameError",
			"result = undefined_var",
			"name 'undefined_var' is not defined",
		},
		{
			"ZeroDivisionError",
			"result = 1 / 0",
			"division by zero",
		},
		{
			"TypeError",
			"result = 'string' + 1",
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

func TestRun_NoResult(t *testing.T) {
	program := serpent.Program[string, string]("input")
	_, err := serpent.Run(program, "test")
	if !errors.Is(err, serpent.ErrNoResult) {
		t.Errorf("expected error: %v; got: %v", serpent.ErrNoResult, err)
	}
}

func TestRun_SlowExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test")
	}

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

func TestRun_FunctionScope(t *testing.T) {
	program := serpent.Program[*struct{}, int](`import math
def calc():
	return int(math.sqrt(4))
result = calc()
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
import os
os.write(fd, b'OK')
os.close(fd)
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
