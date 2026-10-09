// Package lock provides a cross-platform exclusive advisory lock with a bounded
// retry loop and a timeout.
//
// The lock is backed by a persistent .tickets/.lock file that is never deleted,
// which avoids the delete/recreate inode race. Every mutating operation holds
// the lock for the whole read-verify-modify-write cycle.
package lock
