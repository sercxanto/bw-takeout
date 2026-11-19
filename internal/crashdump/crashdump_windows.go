//go:build windows

package crashdump

// Limit changes nothing on Windows and reports that crash dumps are not
// limited (design D3).
func Limit() (Protection, error) { return Protection{}, nil }
