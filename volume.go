package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The last volume lives in a tiny file so lull comes back at the same level.

func volumeFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lull", "volume"), nil
}

func loadVolume() int {
	path, err := volumeFile()
	if err != nil {
		return startVolume
	}
	b, _ := os.ReadFile(path) // a missing file fails Atoi below
	v, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return startVolume
	}
	return min(max(v, 0), 130)
}

func saveVolume(v int) {
	if path, err := volumeFile(); err == nil && os.MkdirAll(filepath.Dir(path), 0o755) == nil {
		_ = os.WriteFile(path, []byte(strconv.Itoa(v)+"\n"), 0o644)
	}
}
