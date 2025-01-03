//go:build darwin

package serpent

import (
	"fmt"
	"path/filepath"
)

// pathPrefix is the search path prefix for finding a Python shared library on macOS systems.
var pathPrefix = []string{
	"/opt/homebrew/Frameworks/Python.framework/Versions/Current/lib",
	"/usr/local/Frameworks/Python.framework/Versions/Current/lib",
}

// findLib attempts to find a Python shared library on macOS systems.
func findLib() (string, error) {
	for _, prefix := range pathPrefix {
		matches, err := filepath.Glob(filepath.Join(prefix, "libpython*.dylib"))
		if err != nil {
			return "", fmt.Errorf("glob: %w", err)
		}
		if len(matches) > 0 {
			return matches[0], nil
		}
	}
	return "", ErrLibraryNotFound
}
