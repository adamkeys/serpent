package serpent_test

import (
	"os"
	"testing"

	"github.com/adamkeys/serpent"
)

func TestLib_LibPythonPath(t *testing.T) {
	const exp = "/path/to/lib.so"
	os.Setenv("LIBPYTHON_PATH", exp)
	defer os.Unsetenv("LIBPYTHON_PATH")

	path, err := serpent.Lib()
	if err != nil {
		t.Fatalf("lib: %v", err)
	}

	if path != exp {
		t.Errorf("unexpected path: %q; got: %q", exp, path)
	}
}

func TestLib_findLib(t *testing.T) {
	path, err := serpent.Lib()
	if err != nil {
		t.Fatalf("lib: %v", err)
	}

	if path == "" {
		t.Error("unexpected library path")
	}
}
