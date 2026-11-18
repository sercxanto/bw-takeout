package run

import (
	"time"

	"bw-takeout/internal/bwcli"

	"bw-takeout/internal/collect"
)

// SetSnapshotHook lets the tests of package run_test see the snapshot of a
// run, to check that its documents were wiped. The returned function
// restores the default.
func SetSnapshotHook(f func(*collect.Snapshot)) (restore func()) {
	snapshotTaken = f
	return func() { snapshotTaken = func(*collect.Snapshot) {} }
}

// LogoutLimitOf returns the logout limit a run uses with d.
func LogoutLimitOf(d Deps) time.Duration { return d.withDefaults().LogoutLimit }

// LogoutResultOf classifies a logout error with a teardown context that the
// limit, an interrupt or nothing ended.
func LogoutResultOf(err error, ended string) (LogoutResult, bwcli.Class) {
	var cause error
	switch ended {
	case "skipped":
		cause = errLogoutSkipped
	case "limit":
		cause = errLogoutLimit
	}
	return logoutResult(err, cause)
}

// SetLogoutReturnedHook lets the tests act between the end of the logout
// and the removal of the profile. f can ask whether a signal was handled.
// The returned function restores the default.
func SetLogoutReturnedHook(f func(interrupted func() bool)) (restore func()) {
	logoutReturned = func(r *runner) {
		f(func() bool {
			r.mu.Lock()
			defer r.mu.Unlock()
			return r.interrupted
		})
	}
	return func() { logoutReturned = func(*runner) {} }
}
