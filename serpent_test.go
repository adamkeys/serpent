package serpent_test

import (
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
	lib, err := serpent.Lib()
	if err != nil {
		t.Fatalf("set LIBPYTHON_PATH: %v", err)
	}
	if err := serpent.Init(lib); err != nil {
		t.Fatalf("init: %v", err)
	}

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
}
