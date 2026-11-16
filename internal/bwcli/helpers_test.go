package bwcli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"bw-takeout/internal/bwcli/bwclitest"
)

// fakePath is the fake bw, built once per package run (design D12).
var fakePath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "bwcli-fake-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fakePath, err = bwclitest.Build(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// Synthetic values used across the tests.
const (
	testItem       = "11111111-2222-4333-8444-555555555555"
	testAttachment = "f00dfeed0123456789abcdef"
	// testKey is a synthetic session key: 88 base64 characters that
	// decode to 64 bytes.
	testKey  = "c3ludGhldGljLXNlc3Npb24ta2V5LXN5bnRoZXRpYy1zZXNzaW9uLWtleS1zeW50aGV0aWMtc2Vzc2lvbi0wMQ=="
	sentinel = "SENTINEL-secret-item-name"
)

type fixture struct {
	t          *testing.T
	data, work string
	client     *Client
	term       *fakeTerminal
}

// newFixture writes the scenario into a fresh data directory and creates a
// client for the fake with a fake terminal that reports no terminal.
func newFixture(t *testing.T, s bwclitest.Scenario, configure ...func(*Config)) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{t: t, data: filepath.Join(root, "data"), work: filepath.Join(root, "work"), term: &fakeTerminal{}}
	for _, d := range []string{f.data, f.work} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := bwclitest.WriteScenario(f.data, s); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Executable: fakePath, DataDir: f.data, WorkDir: f.work, Terminal: f.term}
	for _, fn := range configure {
		fn(&cfg)
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	f.client = c
	return f
}

func scenario(pairs ...any) bwclitest.Scenario {
	s := bwclitest.Scenario{Commands: map[string]bwclitest.Behavior{}}
	for i := 0; i < len(pairs); i += 2 {
		s.Commands[pairs[i].(string)] = pairs[i+1].(bwclitest.Behavior)
	}
	return s
}

func (f *fixture) records() []bwclitest.Record {
	f.t.Helper()
	recs, err := bwclitest.ReadRecords(f.data)
	if err != nil {
		f.t.Fatal(err)
	}
	return recs
}

func (f *fixture) lastRecord() bwclitest.Record {
	f.t.Helper()
	recs := f.records()
	if len(recs) == 0 {
		f.t.Fatal("no bw process was started")
	}
	return recs[len(recs)-1]
}

// wantClass fails the test unless err is a failure of the class.
func wantClass(t *testing.T, err error, class Class) *Error {
	t.Helper()
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("got %v, want a failure of class %s", err, class)
	}
	if e.Class != class {
		t.Fatalf("got class %s (%v), want %s", e.Class, e, class)
	}
	return e
}

// assertNoText fails the test if the failure contains text in its message
// or in any exported field.
func assertNoText(t *testing.T, err error, text string) {
	t.Helper()
	if strings.Contains(err.Error(), text) {
		t.Errorf("failure text %q contains %q", err.Error(), text)
	}
	v := reflect.ValueOf(err).Elem()
	for i := 0; i < v.NumField(); i++ {
		if !v.Type().Field(i).IsExported() {
			continue
		}
		if strings.Contains(fmt.Sprint(v.Field(i).Interface()), text) {
			t.Errorf("field %s contains %q", v.Type().Field(i).Name, text)
		}
	}
}

// fakeTerminal is the Terminal of the tests.
type fakeTerminal struct {
	terminal          bool
	saves, restores   int
	saveErr, restErr  error
	restoredAfterSave bool
	// onRestore runs inside RestoreState, before it returns.
	onRestore func()
}

func (f *fakeTerminal) IsTerminal() bool { return f.terminal }

func (f *fakeTerminal) SaveState() error {
	f.saves++
	return f.saveErr
}

func (f *fakeTerminal) RestoreState() error {
	f.restores++
	f.restoredAfterSave = f.saves > 0
	if f.onRestore != nil {
		f.onRestore()
	}
	return f.restErr
}

var bg = context.Background()

// spawnChild lets the fake start a child where the client stops the whole
// process group. On Windows the child would outlive the test (design D4).
var spawnChild = runtime.GOOS != "windows"
