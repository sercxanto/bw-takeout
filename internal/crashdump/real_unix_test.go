//go:build unix

package crashdump

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

const realHelperEnv = "BW_TAKEOUT_CRASHDUMP_HELPER"

// TestRealLimitHelper is not a test of its own. TestRealLimit starts the
// test binary with it, and it calls the real Limit and prints what it reads
// afterwards (design D6), so that the test process keeps its own limits.
func TestRealLimitHelper(t *testing.T) {
	if os.Getenv(realHelperEnv) == "" {
		t.Skip("helper process for TestRealLimit, runs only when started by it")
	}
	p, err := Limit()
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	fmt.Printf("protection %t %t\n", p.CoreLimit, p.NotDumpable)
	fmt.Println(readLimits())
	os.Exit(0)
}

// readLimits reads the core limit and, on Linux, the dumpable value of the
// calling process, as the lines "core <soft> <hard>" and "dumpable <v>".
func readLimits() string {
	var r unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_CORE, &r); err != nil {
		return "error: " + err.Error()
	}
	s := fmt.Sprintf("core %d %d", r.Cur, r.Max)
	if v, ok := dumpableValue(); ok {
		s += fmt.Sprintf("\ndumpable %d", v)
	}
	return s
}

func TestRealLimit(t *testing.T) {
	var before unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_CORE, &before); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRealLimitHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), realHelperEnv+"=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("helper: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	want := []string{fmt.Sprintf("protection true %t", hasDumpableMark), "core 0 0"}
	if hasDumpableMark {
		want = append(want, "dumpable 0")
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("helper printed\n%s\nwant\n%s", out, strings.Join(want, "\n"))
	}
	var after unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_CORE, &after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("test process core limit changed from %+v to %+v", before, after)
	}
	t.Logf("child: %s; test process keeps core %d %d", strings.Join(lines, "; "), after.Cur, after.Max)
}
