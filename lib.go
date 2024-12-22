package serpent

import (
	"errors"
	"os"
)

// ErrLibraryNotFound is returned when the Python shared library cannot be found.
var ErrLibraryNotFound = errors.New("library not found")

// Lib attempts to find a Python shared library on the system and returns the path if found. If the library
// cannot be found, ErrLibraryNotFound is returned. If the LIBPYTHON_PATH envrionment variable is set, the value
// of that environment variable is returned.
func Lib() (string, error) {
	if path := os.Getenv("LIBPYTHON_PATH"); path != "" {
		return path, nil
	}
	return findLib()
}
