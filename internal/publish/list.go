package publish

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// Leftover is a partial generation found in the target.
type Leftover struct {
	Name string
	// Age is the time since the creation time in the name.
	Age time.Duration
}

// ListPartial returns the partial generations in the target, sorted by
// name, with their age at now. It only reads the directory listing and
// changes nothing (design D9). A partial generation may belong to a run
// that is still in progress, so the caller must not remove it.
func ListPartial(target string, now time.Time) ([]Leftover, error) {
	entries, err := os.ReadDir(target)
	if err != nil {
		return nil, fmt.Errorf("list target: %w", err)
	}
	var out []Leftover
	for _, e := range entries {
		if !e.IsDir() || !strings.HasSuffix(e.Name(), PartialMarker) {
			continue
		}
		created, marker, ok := ParseName(e.Name())
		if !ok || marker != PartialMarker {
			continue
		}
		out = append(out, Leftover{Name: e.Name(), Age: now.Sub(created)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
