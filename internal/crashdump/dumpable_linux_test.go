//go:build linux

package crashdump

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

const hasDumpableMark = true

// fakeDumpable replaces the dumpable calls, so that no unit test marks the
// test process.
func fakeDumpable(t *testing.T, setErr, getErr error, readBack int) {
	t.Helper()
	oldSet, oldGet := setDumpable, getDumpable
	t.Cleanup(func() { setDumpable, getDumpable = oldSet, oldGet })
	setDumpable = func(v int) error {
		if v != 0 {
			t.Errorf("dumpable set to %d", v)
		}
		return setErr
	}
	getDumpable = func() (int, error) { return readBack, getErr }
}

func TestDumpableErrors(t *testing.T) {
	boom := errors.New("prctl 4 failed")
	cases := []struct {
		name           string
		setErr, getErr error
		readBack       int
	}{
		{"set fails", boom, nil, 0},
		{"get fails", nil, boom, 0},
		{"reads back as 1", nil, nil, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeRlimit(t, nil, nil, unix.Rlimit{})
			fakeDumpable(t, c.setErr, c.getErr, c.readBack)
			p, err := Limit()
			if !errors.Is(err, ErrDumpable) {
				t.Fatalf("error %v, want ErrDumpable", err)
			}
			if p != (Protection{}) {
				t.Errorf("protection %+v after a failure", p)
			}
			noDetail(t, err)
		})
	}
}

// dumpableValue reads the dumpable value of the calling process.
func dumpableValue() (int, bool) {
	v, err := unix.PrctlRetInt(unix.PR_GET_DUMPABLE, 0, 0, 0, 0)
	return v, err == nil
}
