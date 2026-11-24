package capture

import "testing"

// Every type-specific object of typeObjectKeys is a walked object of the
// item set, so that the two lists cannot drift apart.
func TestItemKeysHoldEveryTypeObject(t *testing.T) {
	for typ, key := range typeObjectKeys {
		n, ok := itemKeys[key]
		if !ok || n == nil || n.array || len(n.shape) == 0 {
			t.Errorf("type %d: object %q is not a walked object of the item keys", typ, key)
		}
	}
	for doc, s := range documentKeys {
		if n := s["items"]; n == nil || !n.array {
			t.Errorf("%s: items are not walked", doc)
		}
	}
}
