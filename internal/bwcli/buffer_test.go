package bwcli

import (
	"bytes"
	"testing"
)

// The buffer clears every array it leaves behind when it grows (design D13
// of capture-collection).
func TestBufferGrowthClearsOldArrays(t *testing.T) {
	b := &limitedBuffer{limit: 1 << 20}
	var want []byte
	var left [][]byte
	piece := []byte("secret-piece-")
	for i := 0; i < 2000; i++ {
		before := b.buf[:cap(b.buf)]
		if _, err := b.Write(piece); err != nil {
			t.Fatal(err)
		}
		want = append(want, piece...)
		if len(before) > 0 && &before[0] != &b.buf[0] {
			left = append(left, before)
		}
	}
	if !bytes.Equal(b.buf, want) {
		t.Fatal("buffer content differs from what was written")
	}
	if len(left) < 3 {
		t.Fatalf("the buffer grew only %d times; the test needs several", len(left))
	}
	for i, old := range left {
		for _, x := range old {
			if x != 0 {
				t.Fatalf("array %d left behind by growth holds output", i)
			}
		}
	}
	b.wipe()
	if b.buf != nil {
		t.Error("wipe kept the buffer")
	}
}

// Growth never allocates more than the limit, and a write past the limit is
// refused.
func TestBufferGrowthStaysWithinTheLimit(t *testing.T) {
	exceeded := 0
	b := &limitedBuffer{limit: 10000, onExceed: func() { exceeded++ }}
	if _, err := b.Write(make([]byte, 9000)); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write(make([]byte, 1000)); err != nil {
		t.Fatal(err)
	}
	if cap(b.buf) > 10000 {
		t.Errorf("capacity %d passes the limit", cap(b.buf))
	}
	if _, err := b.Write([]byte{1}); err == nil || !b.exceeded || exceeded != 1 {
		t.Errorf("write past the limit: err %v, exceeded %v, onExceed %d", err, b.exceeded, exceeded)
	}
}
