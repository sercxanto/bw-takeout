//go:build unix

package crashdump

import (
	"errors"
	"regexp"
	"testing"

	"golang.org/x/sys/unix"
)

// fakeRlimit replaces the core limit calls. setErr and getErr fail the
// calls, and readBack is what the get returns.
func fakeRlimit(t *testing.T, setErr, getErr error, readBack unix.Rlimit) {
	t.Helper()
	oldSet, oldGet := setRlimit, getRlimit
	t.Cleanup(func() { setRlimit, getRlimit = oldSet, oldGet })
	setRlimit = func(resource int, _ *unix.Rlimit) error {
		if resource != unix.RLIMIT_CORE {
			t.Errorf("set of resource %d", resource)
		}
		return setErr
	}
	getRlimit = func(resource int, r *unix.Rlimit) error {
		if resource != unix.RLIMIT_CORE {
			t.Errorf("get of resource %d", resource)
		}
		*r = readBack
		return getErr
	}
}

func TestCoreLimitErrors(t *testing.T) {
	boom := errors.New("operation not permitted by /etc/limits 42")
	cases := []struct {
		name           string
		setErr, getErr error
		readBack       unix.Rlimit
	}{
		{"set fails", boom, nil, unix.Rlimit{}},
		{"get fails", nil, boom, unix.Rlimit{}},
		{"soft limit reads back as 1", nil, nil, unix.Rlimit{Cur: 1, Max: 0}},
		{"hard limit reads back as 1", nil, nil, unix.Rlimit{Cur: 0, Max: 1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeRlimit(t, c.setErr, c.getErr, c.readBack)
			fakeDumpable(t, nil, nil, 0)
			p, err := Limit()
			if !errors.Is(err, ErrCoreLimit) {
				t.Fatalf("error %v, want ErrCoreLimit", err)
			}
			if p != (Protection{}) {
				t.Errorf("protection %+v after a failure", p)
			}
			noDetail(t, err)
		})
	}
}

func TestLimitSucceeds(t *testing.T) {
	fakeRlimit(t, nil, nil, unix.Rlimit{})
	fakeDumpable(t, nil, nil, 0)
	p, err := Limit()
	if err != nil {
		t.Fatal(err)
	}
	want := Protection{CoreLimit: true, NotDumpable: hasDumpableMark}
	if p != want {
		t.Errorf("protection %+v, want %+v", p, want)
	}
}

// noDetail checks that an error text holds no number and no path.
func noDetail(t *testing.T, err error) {
	t.Helper()
	if regexp.MustCompile(`[0-9/\\]`).MatchString(err.Error()) {
		t.Errorf("error text %q holds a number or a path", err)
	}
}
