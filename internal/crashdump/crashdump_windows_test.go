//go:build windows

package crashdump

import "testing"

// TestLimitOnWindows calls the real stub, which changes nothing.
func TestLimitOnWindows(t *testing.T) {
	p, err := Limit()
	if err != nil {
		t.Fatal(err)
	}
	if p != (Protection{}) {
		t.Errorf("protection %+v, want none", p)
	}
}
