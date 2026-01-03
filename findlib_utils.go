package serpent

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// preferredVersion sorts library paths and returns the highest version.
func preferredVersion(paths []string) string {
	if len(paths) == 1 {
		return paths[0]
	}
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))
	return paths[0]
}

// pkgConfigLibPath attempts to find the Python library using pkg-config.
// It tries python3-embed first (for static linking), then python3.
func pkgConfigLibPath(libExtension string) (string, bool) {
	libDir, ok := pkgConfigGetLibDir("python3")
	if !ok {
		return "", false
	}

	// Search for the actual library file in the directory
	pattern := filepath.Join(libDir, "libpython*"+libExtension)
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", false
	}

	return preferredVersion(matches), true
}

// pkgConfigGetLibDir runs pkg-config --libs and extracts the -L path.
func pkgConfigGetLibDir(pkg string) (string, bool) {
	cmd := exec.Command("pkg-config", "--libs", pkg)
	output, err := cmd.Output()
	if err != nil {
		return "", false
	}

	parts := strings.Fields(string(output))
	for _, part := range parts {
		if strings.HasPrefix(part, "-L") {
			return strings.TrimPrefix(part, "-L"), true
		}
	}

	cmd = exec.Command("pkg-config", "--variable=libdir", pkg)
	output, err = cmd.Output()
	if err != nil {
		return "", false
	}

	libDir := strings.TrimSpace(string(output))
	if libDir != "" {
		return libDir, true
	}

	return "", false
}

// fileExists returns true if the given path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}
