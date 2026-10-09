package lock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// DefaultTimeout is the bounded wait for the exclusive lock before a clear
// timeout error is returned. A caller that times out has made no changes
// (spec 6.1).
const DefaultTimeout = 5 * time.Second

// retryInterval is how often the non-blocking try is repeated while waiting.
// It is deliberately small so a released lock is picked up promptly without
// busy-spinning.
const retryInterval = 20 * time.Millisecond

// ErrTimeout is returned when the exclusive lock could not be acquired within
// the bounded timeout. The caller must make no changes.
var ErrTimeout = errors.New("timed out waiting for the backlog lock")

// ErrNotHeld is returned when Release is called on a Lock that is not held.
var ErrNotHeld = errors.New("attempted to release a lock that is not held")

// IsTimeout reports whether err is or wraps ErrTimeout.
func IsTimeout(err error) bool { return errors.Is(err, ErrTimeout) }

// Lock is an acquired exclusive advisory lock. It is not safe for concurrent
// use by multiple goroutines; one goroutine owns one Lock.
type Lock struct {
	flock *flock.Flock
	held  bool
}

// Acquire opens or creates the persistent lock file at path and takes an
// exclusive advisory lock, retrying until timeout elapses. The lock file is
// never deleted, which avoids the delete/recreate inode race (spec 6.1).
//
// A timeout returns ErrTimeout and guarantees that no lock is held. The lock
// file itself may be created; that is side-effect free for the backlog.
func Acquire(path string, timeout time.Duration) (*Lock, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}

	l := &Lock{flock: flock.New(path)}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ok, err := l.flock.TryLockContext(ctx, retryInterval)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w after %s", ErrTimeout, timeout)
		}
		return nil, fmt.Errorf("acquire lock %s: %w", path, err)
	}
	if !ok {
		return nil, fmt.Errorf("%w after %s", ErrTimeout, timeout)
	}
	l.held = true
	return l, nil
}

// Release drops the exclusive lock. It is idempotent-like in that a second call
// returns ErrNotHeld rather than silently succeeding, so a double release is
// visible. The lock file is left in place.
func (l *Lock) Release() error {
	if l == nil || !l.held {
		return ErrNotHeld
	}
	err := l.flock.Unlock()
	l.held = false
	if err != nil {
		return fmt.Errorf("release lock: %w", err)
	}
	return nil
}

// Path returns the lock file path.
func (l *Lock) Path() string {
	if l == nil || l.flock == nil {
		return ""
	}
	return l.flock.Path()
}

// Held reports whether the lock is currently held.
func (l *Lock) Held() bool {
	return l != nil && l.held
}
