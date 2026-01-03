//go:build darwin

package serpent

import (
	"os"
	"path/filepath"
)

// platformSupportsSubInterpreters indicates whether the current platform supports Python sub-interpreters.
var platformSupportsSubInterpreters = true

// searchPaths returns the list of paths to search for Python shared libraries on macOS.
func searchPaths() []string {
	paths := []string{
		"/opt/homebrew/Frameworks/Python.framework/Versions/*/lib",
		"/usr/local/Frameworks/Python.framework/Versions/*/lib",
		"/Library/Frameworks/Python.framework/Versions/*/lib",
		"/opt/local/Library/Frameworks/Python.framework/Versions/*/lib",
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

// findLib attempts to find a Python shared library on macOS systems.
// It first tries pkg-config, then falls back to searching common paths.
func findLib() (string, error) {
	if path, ok := pkgConfigLibPath(".dylib"); ok {
		return path, nil
	}

	for _, prefix := range searchPaths() {
		dirMatches, err := filepath.Glob(prefix)
		if err != nil {
			continue
		}

		for _, dir := range dirMatches {
			matches, err := filepath.Glob(filepath.Join(dir, "libpython*.dylib"))
			if err != nil {
				continue
			}
			if len(matches) > 0 {
				return preferredVersion(matches), nil
			}
		}
	}
	return "", ErrLibraryNotFound
}
