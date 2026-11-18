package offline

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// forbidden reports whether the offline command must not depend on the
// package (design D9 of interactive-backup-run): no network, no process
// start, and none of the packages that start bw, collect, judge a capture
// or run a backup. verify is excluded because its key parsing pulls in the
// network package.
func forbidden(pkg string) bool {
	switch pkg {
	case "net", "os/exec",
		"bw-takeout/internal/verify",
		"bw-takeout/internal/bwcli",
		"bw-takeout/internal/collect",
		"bw-takeout/internal/run":
		return true
	}
	return strings.HasPrefix(pkg, "net/")
}

// TestNoNetworkOrProcessPackage checks the dependencies of this package,
// which is all the verify command runs. It replaces the check of the whole
// binary, which now also holds the backup command.
func TestNoNetworkOrProcessPackage(t *testing.T) {
	out, err := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "list", "-deps", "bw-takeout/internal/offline").Output()
	if err != nil {
		t.Fatal(err)
	}
	deps := strings.Fields(string(out))
	if len(deps) < 10 {
		t.Fatalf("dependency list too short: %v", deps)
	}
	for _, d := range deps {
		if forbidden(d) {
			t.Errorf("the offline command depends on %s", d)
		}
	}
}

func TestForbiddenPackages(t *testing.T) {
	for _, p := range []string{"net", "net/http", "net/netip", "os/exec", "bw-takeout/internal/verify", "bw-takeout/internal/bwcli", "bw-takeout/internal/collect", "bw-takeout/internal/run"} {
		if !forbidden(p) {
			t.Errorf("%s is not forbidden", p)
		}
	}
	for _, p := range []string{"os", "network", "bw-takeout/internal/archive", "bw-takeout/internal/runner", "bw-takeout/internal/verifyx"} {
		if forbidden(p) {
			t.Errorf("%s is forbidden", p)
		}
	}
}
