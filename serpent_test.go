package serpent_test

import (
	"testing"

	"github.com/adamkeys/serpent"
)

func TestRun(t *testing.T) {
	err := serpent.Init("/usr/local/Cellar/python3.9/3.9.0_5/Frameworks/Python.framework/Versions/3.9/lib/libpython3.9.dylib")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	defer serpent.Finalize()

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
