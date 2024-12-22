//go:build linux

package serpent

import (
	"fmt"
	"path/filepath"
)

// pathPrefix is the search path prefix for finding a Python shared library on Linux systems.
// TODO: Find path on other architectures.
const pathPrefix = "/usr/lib/x86_64-linux-gnu"

// findLib attempts to find a Python shared library on macOS systems.
func findLib() (string, error) {
	matches, err := filepath.Glob(filepath.Join(pathPrefix, "libpython*.so"))
	if err != nil {
		return "", fmt.Errorf("glob: %w", err)
	}
	if len(matches) == 0 {
		return "", ErrLibraryNotFound
	}

	return matches[0], nil
}
