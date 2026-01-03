//go:build linux

package serpent

import (
	"os"
	"path/filepath"
	"runtime"
)

// platformSupportsSubInterpreters indicates whether the current platform supports Python sub-interpreters.
var platformSupportsSubInterpreters = true

// archLibDirs maps GOARCH to the corresponding library directory names on Linux.
var archLibDirs = map[string][]string{
	"amd64": {"x86_64-linux-gnu", "lib64"},
	"arm64": {"aarch64-linux-gnu", "lib64"},
	"arm":   {"arm-linux-gnueabihf", "arm-linux-gnueabi"},
	"386":   {"i386-linux-gnu", "i686-linux-gnu", "lib32"},
}

// searchPaths returns the list of paths to search for Python shared libraries on Linux.
func searchPaths() []string {
	var paths []string
	if dirs, ok := archLibDirs[runtime.GOARCH]; ok {
		for _, dir := range dirs {
			paths = append(paths, filepath.Join("/usr/lib", dir))
		}
	}
	paths = append(paths,
		"/usr/lib64",
		"/usr/lib",
		"/usr/local/lib",
	)
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

// findLib attempts to find a Python shared library on Linux systems.
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
