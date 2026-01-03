//go:build unix && !(darwin || linux)

package serpent

import (
	"os"
	"path/filepath"
)

// searchPaths returns the list of paths to search for Python shared libraries on Unix systems.
func searchPaths() []string {
	paths := []string{
		"/usr/local/lib",
		"/usr/lib",
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths,
			filepath.Join(home, ".pyenv/versions/*/lib"),
			filepath.Join(home, "miniconda3/lib"),
			filepath.Join(home, "anaconda3/lib"),
			filepath.Join(home, ".local/lib"),
		)
	}

	return paths
}

// findLib attempts to find a Python shared library on Unix systems.
// It first tries pkg-config, then falls back to searching common paths.
func findLib() (string, error) {
	if path, ok := pkgConfigLibPath(".so"); ok {
		return path, nil
	}

	for _, prefix := range searchPaths() {
		matches, err := filepath.Glob(filepath.Join(prefix, "libpython*.so"))
		if err != nil {
			continue
		}
		if len(matches) > 0 {
			return preferredVersion(matches), nil
		}
	}
	return "", ErrLibraryNotFound
}
