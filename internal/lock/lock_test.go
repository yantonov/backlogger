package lock_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yantonov/backlogger/internal/lock"
)

func mustAcquire(t *testing.T, path string) *lock.Lock {
	t.Helper()
	l, err := lock.Acquire(path, time.Second)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	return l
}

func TestAcquire_FreeLock_IsHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")

	l, err := lock.Acquire(path, time.Second)

	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer func() { _ = l.Release() }()
	if !l.Held() {
		t.Fatal("lock should be held after Acquire")
	}
}

func TestRelease_HeldLock_IsNoLongerHeldAndKeepsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")
	l := mustAcquire(t, path)

	err := l.Release()

	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if l.Held() {
		t.Fatal("lock should not be held after Release")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file must persist after release: %v", err)
	}
}

// TestAcquire_AfterRelease_Succeeds proves releasing a lock lets a later
// acquirer proceed.
func TestAcquire_AfterRelease_Succeeds(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")
	if err := mustAcquire(t, path).Release(); err != nil {
		t.Fatal(err)
	}

	second, err := lock.Acquire(path, time.Second)

	if err != nil {
		t.Fatalf("second acquire after release: %v", err)
	}
	if err := second.Release(); err != nil {
		t.Fatalf("second release: %v", err)
	}
}

// TestRelease_ReleasedLock_ReturnsErrNotHeld proves a double release does not
// silently succeed.
func TestRelease_ReleasedLock_ReturnsErrNotHeld(t *testing.T) {
	l := mustAcquire(t, filepath.Join(t.TempDir(), ".lock"))
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}

	err := l.Release()

	if err != lock.ErrNotHeld {
		t.Fatalf("expected ErrNotHeld on double release, got %v", err)
	}
}
