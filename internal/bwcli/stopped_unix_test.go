//go:build unix

package bwcli

import (
	"errors"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// assertStopped fails the test unless every process ID belongs to a process
// that has ended. A zombie that waits for its parent counts as ended.
func assertStopped(t *testing.T, pids ...int) {
	t.Helper()
	for _, pid := range pids {
		if pid == 0 {
			t.Error("process ID not recorded")
			continue
		}
		deadline := time.Now().Add(5 * time.Second)
		for !ended(pid) {
			if time.Now().After(deadline) {
				t.Errorf("process %d still runs", pid)
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func ended(pid int) bool {
	err := syscall.Kill(pid, 0)
	if errors.Is(err, syscall.ESRCH) {
		return true
	}
	if runtime.GOOS == "linux" {
		b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if err != nil {
			return true
		}
		s := string(b)
		if i := strings.LastIndexByte(s, ')'); i >= 0 && len(s) > i+2 {
			return s[i+2] == 'Z'
		}
	}
	return false
}
