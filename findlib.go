//go:build !(linux || darwin)

package serpent

// platformSupportsSubInterpreters indicates whether the current platform supports Python sub-interpreters.
var platformSupportsSubInterpreters = false

// findLib returns ErrLibraryNotFound on systems which do not support the library search.
func findLib() (string, error) {
	return "", ErrLibraryNotFound
}
