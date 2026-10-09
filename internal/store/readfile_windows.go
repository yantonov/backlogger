//go:build windows

package store

import (
	"io"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// readFile reads path allowing other processes to replace or delete the file
// while it is open. Go's default os.ReadFile opens without FILE_SHARE_DELETE,
// which makes a concurrent os.Rename onto the same path fail with "Access is
// denied" on Windows. bl's own readers therefore opt into full sharing so a
// writer can always replace the file atomically.
func readFile(path string) ([]byte, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	h, err := windows.CreateFile(
		p,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	f := os.NewFile(uintptr(h), path)
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

// replaceFile moves src onto dst. On Windows a concurrent reader that did not
// opt into FILE_SHARE_DELETE can make the replace fail with a sharing
// violation, so it retries briefly. The final error is returned unchanged.
func replaceFile(src, dst string) error {
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		err := os.Rename(src, dst)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(5 * time.Millisecond)
	}
}
