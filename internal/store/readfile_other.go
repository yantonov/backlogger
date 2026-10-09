//go:build !windows

package store

import "os"

// readFile reads path. On POSIX no extra sharing flags are needed: os.Rename
// replaces the directory entry while open readers keep the old inode.
func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func replaceFile(src, dst string) error {
	return os.Rename(src, dst)
}
