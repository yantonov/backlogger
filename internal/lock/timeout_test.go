package lock_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yantonov/backlogger/internal/lock"
)

// TestAcquire_HeldLock_TimesOutAndLeavesFilesUntouched proves that a second
// acquirer times out with a clean error and that the timeout path changes no
// file contents.
func TestAcquire_HeldLock_TimesOutAndLeavesFilesUntouched(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".lock")
	payload := filepath.Join(dir, "payload.txt")
	if err := os.WriteFile(payload, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	held, err := lock.Acquire(lockPath, time.Second)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer func() { _ = held.Release() }()

	start := time.Now()
	second, err := lock.Acquire(lockPath, 150*time.Millisecond)
	if err == nil {
		_ = second.Release()
		t.Fatal("second acquire unexpectedly succeeded while the lock was held")
	}
	if !lock.IsTimeout(err) {
		t.Fatalf("expected a timeout error, got %v", err)
	}
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("timeout returned too early: %s", elapsed)
	}

	got, err := os.ReadFile(payload)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Fatalf("payload changed on the timeout path: %q", got)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("persistent lock file should remain: %v", err)
	}
}

// TestAcquire_HeldLock_ReturnsErrTimeout checks the error can be matched with
// errors.Is.
func TestAcquire_HeldLock_ReturnsErrTimeout(t *testing.T) {
	dir := t.TempDir()
	held, err := lock.Acquire(filepath.Join(dir, ".lock"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Release() }()

	_, err = lock.Acquire(filepath.Join(dir, ".lock"), 20*time.Millisecond)
	if !errors.Is(err, lock.ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
}
