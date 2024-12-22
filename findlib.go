//go:build !(darwin || linux)

package serpent

// findLib returns ErrLibraryNotFound on systems which do not support the library search.
func findLib() (string, error) {
	return "", ErrLibraryNotFound
}
