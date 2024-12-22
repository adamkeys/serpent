//go:build darwin

package serpent

import (
	"fmt"
	"path/filepath"
)

// pathPrefix is the search path prefix for finding a Python shared library on macOS systems.
const pathPrefix = "/usr/local/Frameworks/Python.framework/Versions/Current/lib"

// findLib attempts to find a Python shared library on macOS systems.
func findLib() (string, error) {
	matches, err := filepath.Glob(filepath.Join(pathPrefix, "libpython*.dylib"))
	if err != nil {
		return "", fmt.Errorf("glob: %w", err)
	}
	if len(matches) == 0 {
		return "", ErrLibraryNotFound
	}

	return matches[0], nil
}
