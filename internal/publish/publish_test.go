package publish

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"filippo.io/age"

	"bitwarden-backup/internal/archive"
	"bitwarden-backup/internal/archive/archivetest"
	"bitwarden-backup/internal/capture/capturetest"
	"bitwarden-backup/internal/verify"
)

var testNow = time.Date(2026, 11, 14, 17, 0, 0, 0, time.UTC)

func fixedClock() time.Time { return testNow }

// sameFirst returns a random source whose first value is first and whose
// later values are random.
func sameFirst(first string) io.Reader {
	b, err := hex.DecodeString(first)
	if err != nil || len(b) != 4 {
		panic("bad first value")
	}
	return io.MultiReader(bytes.NewReader(b), rand.Reader)
}

// sequence returns a random source that yields the given values in order.
func sequence(values ...string) io.Reader {
	var b []byte
	for _, v := range values {
		d, err := hex.DecodeString(v)
		if err != nil || len(d) != 4 {
			panic("bad value")
		}
		b = append(b, d...)
	}
	return bytes.NewReader(b)
}

func baseName(random string) string {
	return NamePrefix + testNow.Format(timeLayout) + "-" + random
}

type capture struct {
	f            *capturetest.Fixture
	verification []byte
}

func captureWith(t *testing.T, want verify.Verdict) capture {
	t.Helper()
	var f *capturetest.Fixture
	switch want {
	case verify.Verified:
		f = capturetest.Intact(t)
	case verify.Unverified:
		f = capturetest.VariantByName(t, capturetest.BlobLoginItem).Build(t)
	case verify.Failed:
		f = capturetest.VariantByName(t, capturetest.MissingItem).Build(t)
	}
	v, got := archivetest.Verification(t, f, true)
	if got != want {
		t.Fatalf("precondition: fixture verdict is %s, want %s", got, want)
	}
	return capture{f: f, verification: v}
}

// publishCapture runs the whole flow: begin, write the archive, commit. A
// write error aborts the generation. A commit error leaves it as it is.
func publishCapture(t *testing.T, p *Publisher, target string, c capture, recipients ...string) (*Published, error) {
	t.Helper()
	g, err := p.Begin(target)
	if err != nil {
		return nil, err
	}
	if err := writeArchive(t, g.Writer(), c, recipients...); err != nil {
		if aerr := g.Abort(); aerr != nil {
			t.Errorf("abort: %v", aerr)
		}
		return nil, err
	}
	return g.Commit(c.verification)
}

func writeArchive(t *testing.T, w io.Writer, c capture, recipients ...string) error {
	t.Helper()
	aw, err := archive.NewWriter(w, recipients, archivetest.Created, archivetest.Documents(t, c.f))
	if err != nil {
		return err
	}
	for _, a := range archivetest.Attachments(c.f) {
		if err := aw.AddAttachment(a); err != nil {
			return err
		}
	}
	return aw.Finish(archivetest.Meta(c.verification))
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

// snapshot hashes every file and records every directory below root.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			out[rel] = "dir"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// populated returns a target with older generations and unrelated files.
func populated(t *testing.T) string {
	t.Helper()
	target := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(target, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(NamePrefix+"20260101T000000Z-00000001/"+ArchiveName, "older verified generation")
	write(NamePrefix+"20260102T000000Z-00000002"+UnverifiedMarker+"/"+ArchiveName, "older unverified generation")
	write(NamePrefix+"20260103T000000Z-00000003"+PartialMarker+"/"+ArchiveName, "leftover")
	write("notes.txt", "unrelated file")
	write("other/nested.bin", "unrelated nested file")
	return target
}

func unchanged(t *testing.T, target string, before map[string]string) {
	t.Helper()
	after := snapshot(t, target)
	for path, sum := range before {
		if after[path] != sum {
			t.Errorf("%s changed or vanished", path)
		}
	}
}

// newEntries returns the names in target that are not in before.
func newEntries(t *testing.T, target string, before map[string]string) []string {
	t.Helper()
	var out []string
	for _, n := range names(t, target) {
		if _, ok := before[n]; !ok {
			out = append(out, n)
		}
	}
	return out
}

func verifyGeneration(t *testing.T, dir string, id age.Identity) {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, ArchiveName))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := archive.Read(f, []age.Identity{id}, nil); err != nil {
		t.Errorf("%s does not verify: %v", filepath.Base(dir), err)
	}
}

func TestVerdictDecidesPublication(t *testing.T) {
	id := archivetest.NewKey(t)
	cases := []struct {
		verdict verify.Verdict
		want    string
		wantErr error
	}{
		{verify.Verified, baseName("0000000a"), nil},
		{verify.Unverified, baseName("0000000a") + UnverifiedMarker, nil},
		{verify.Failed, "", ErrFailedCapture},
	}
	for _, c := range cases {
		t.Run(string(c.verdict), func(t *testing.T) {
			target := t.TempDir()
			p := &Publisher{Now: fixedClock, Rand: sameFirst("0000000a")}
			pub, err := publishCapture(t, p, target, captureWith(t, c.verdict), id.Recipient().String())
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("got %v, want %v", err, c.wantErr)
			}
			got := names(t, target)
			if c.want == "" {
				if len(got) != 0 {
					t.Fatalf("target holds %v", got)
				}
				return
			}
			if len(got) != 1 || got[0] != c.want || pub.Name != c.want || pub.Verdict != string(c.verdict) {
				t.Fatalf("target holds %v, published %+v", got, pub)
			}
			if inner := names(t, pub.Path); len(inner) != 1 || inner[0] != ArchiveName {
				t.Errorf("generation holds %v", inner)
			}
			verifyGeneration(t, pub.Path, id)
		})
	}
}

func TestUnknownVerdictIsNotPublished(t *testing.T) {
	target := t.TempDir()
	id := archivetest.NewKey(t)
	c := captureWith(t, verify.Verified)
	c.verification = []byte(`{"verdict":"perhaps"}`)
	_, err := publishCapture(t, &Publisher{Now: fixedClock}, target, c, id.Recipient().String())
	if !errors.Is(err, ErrUnknownVerdict) || len(names(t, target)) != 0 {
		t.Fatalf("got %v, target %v", err, names(t, target))
	}
}

func TestNoNamesFromTheVault(t *testing.T) {
	target := t.TempDir()
	id := archivetest.NewKey(t)
	c := captureWith(t, verify.Verified)
	if _, err := publishCapture(t, &Publisher{}, target, c, id.Recipient().String()); err != nil {
		t.Fatal(err)
	}
	all := snapshot(t, target)
	ids := []string{capturetest.ItemPasskeyLogin, capturetest.ItemPlainLogin, capturetest.ItemNote,
		capturetest.AttachmentFirst, capturetest.AttachmentSecond, capturetest.Marker}
	for path := range all {
		for _, s := range ids {
			if strings.Contains(path, s) {
				t.Errorf("%s contains %s", path, s)
			}
		}
	}
	if len(all) != 3 { // ".", the generation directory and the archive
		t.Errorf("target holds %v", all)
	}
}

func TestTwoGenerationsInTheSameSecond(t *testing.T) {
	target := t.TempDir()
	id := archivetest.NewKey(t)
	c := captureWith(t, verify.Verified)
	// Both runs see the same clock value and the same first random value.
	for i := 0; i < 2; i++ {
		p := &Publisher{Now: fixedClock, Rand: sameFirst("0000000b")}
		if _, err := publishCapture(t, p, target, c, id.Recipient().String()); err != nil {
			t.Fatal(err)
		}
	}
	got := names(t, target)
	if len(got) != 2 || got[0] == got[1] {
		t.Fatalf("target holds %v", got)
	}
	for _, n := range got {
		created, marker, ok := ParseName(n)
		if !ok || marker != "" || !created.Equal(testNow) {
			t.Errorf("name %s", n)
		}
	}
}

func TestNameCollision(t *testing.T) {
	for _, existing := range []string{"", UnverifiedMarker, PartialMarker} {
		t.Run("existing"+existing, func(t *testing.T) {
			target := t.TempDir()
			taken := filepath.Join(target, baseName("0000000c")+existing)
			if err := os.Mkdir(taken, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(taken, ArchiveName), []byte("existing"), 0o600); err != nil {
				t.Fatal(err)
			}
			before := snapshot(t, target)
			id := archivetest.NewKey(t)
			p := &Publisher{Now: fixedClock, Rand: sequence("0000000c", "0000000d")}
			pub, err := publishCapture(t, p, target, captureWith(t, verify.Verified), id.Recipient().String())
			if err != nil {
				t.Fatal(err)
			}
			if pub.Name != baseName("0000000d") {
				t.Errorf("published as %s", pub.Name)
			}
			unchanged(t, target, before)
			verifyGeneration(t, pub.Path, id)
		})
	}
}

func TestNoFreeName(t *testing.T) {
	target := t.TempDir()
	if err := os.Mkdir(filepath.Join(target, baseName("0000000e")+PartialMarker), 0o700); err != nil {
		t.Fatal(err)
	}
	values := make([]string, maxAttempts)
	for i := range values {
		values[i] = "0000000e"
	}
	p := &Publisher{Now: fixedClock, Rand: sequence(values...)}
	if _, err := p.Begin(target); !errors.Is(err, ErrNoFreeName) {
		t.Fatalf("got %v", err)
	}
	if got := names(t, target); len(got) != 1 {
		t.Errorf("target holds %v", got)
	}
}

func TestSuccessfulRunNextToOlderGenerations(t *testing.T) {
	target := populated(t)
	before := snapshot(t, target)
	id := archivetest.NewKey(t)
	pub, err := publishCapture(t, &Publisher{Now: fixedClock}, target, captureWith(t, verify.Verified), id.Recipient().String())
	if err != nil {
		t.Fatal(err)
	}
	unchanged(t, target, before)
	if added := newEntries(t, target, before); len(added) != 1 || added[0] != pub.Name {
		t.Errorf("added %v", added)
	}
}

// fault is one injected failure: the nth call of op fails with err.
type fault struct {
	op  string
	nth int
	err error
	// writeAfter, when positive, makes writes fail after that many bytes.
	writeAfter int
}

// faultFS is the real file system with one injected failure.
type faultFS struct {
	osFS
	fault fault
	mu    sync.Mutex
	calls map[string]int
}

func (f *faultFS) hit(op string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[op]++
	if f.fault.op == op && f.calls[op] == f.fault.nth {
		return f.fault.err
	}
	return nil
}

func (f *faultFS) Stat(name string) (fs.FileInfo, error) {
	if err := f.hit("stat"); err != nil {
		return nil, err
	}
	return f.osFS.Stat(name)
}

func (f *faultFS) Mkdir(name string, perm fs.FileMode) error {
	if err := f.hit("mkdir"); err != nil {
		return err
	}
	return f.osFS.Mkdir(name, perm)
}

func (f *faultFS) CreateExclusive(name string, perm fs.FileMode) (file, error) {
	if err := f.hit("create"); err != nil {
		return nil, err
	}
	inner, err := f.osFS.CreateExclusive(name, perm)
	if err != nil {
		return nil, err
	}
	return &faultFile{file: inner, fs: f}, nil
}

func (f *faultFS) SyncDir(name string) error {
	if err := f.hit("syncdir"); err != nil {
		return err
	}
	return f.osFS.SyncDir(name)
}

func (f *faultFS) Rename(oldpath, newpath string) error {
	if err := f.hit("rename"); err != nil {
		return err
	}
	return f.osFS.Rename(oldpath, newpath)
}

func (f *faultFS) Remove(name string) error {
	if err := f.hit("remove"); err != nil {
		return err
	}
	return f.osFS.Remove(name)
}

type faultFile struct {
	file
	fs      *faultFS
	written int
}

func (f *faultFile) Write(p []byte) (int, error) {
	if limit := f.fs.fault.writeAfter; limit > 0 && f.written+len(p) > limit {
		n, _ := f.file.Write(p[:limit-f.written])
		f.written += n
		return n, f.fs.fault.err
	}
	n, err := f.file.Write(p)
	f.written += n
	return n, err
}

func (f *faultFile) Sync() error {
	if err := f.fs.hit("sync"); err != nil {
		return err
	}
	return f.file.Sync()
}

func (f *faultFile) Close() error {
	if err := f.fs.hit("close"); err != nil {
		f.file.Close()
		return err
	}
	return f.file.Close()
}

var errInjected = errors.New("injected failure")

// causedBy reports whether err comes from injected. age reports a failed
// header write with the cause in its message but not in its error chain.
func causedBy(err, injected error) bool {
	return errors.Is(err, injected) || (err != nil && strings.Contains(err.Error(), injected.Error()))
}

// The steps of design D8 in the order a run calls them. Stat is called for
// the target, twice for the final names after the partial directory was
// created, and once before the rename.
var steps = []fault{
	{op: "stat", nth: 1},          // 1. check the target
	{op: "mkdir", nth: 1},         // 2. create the partial directory
	{op: "stat", nth: 2},          // 2. check the final names after mkdir
	{op: "create", nth: 1},        // 3. create the archive file
	{op: "write", writeAfter: 50}, // 3. write the encrypted stream
	{op: "sync", nth: 1},          // 4. flush the archive file
	{op: "close", nth: 1},         // 4. close it
	{op: "syncdir", nth: 1},       // 4. flush the generation directory
	{op: "stat", nth: 4},          // 5. check the final name
	{op: "rename", nth: 1},        // 5. rename
	{op: "syncdir", nth: 2},       // 5. flush the target directory
}

func (f fault) String() string {
	if f.writeAfter > 0 {
		return fmt.Sprintf("%s after %d bytes", f.op, f.writeAfter)
	}
	return fmt.Sprintf("%s #%d", f.op, f.nth)
}

func TestFailingRunNextToOlderGenerations(t *testing.T) {
	id := archivetest.NewKey(t)
	c := captureWith(t, verify.Verified)
	for _, step := range steps {
		t.Run(step.String(), func(t *testing.T) {
			target := populated(t)
			before := snapshot(t, target)
			step.err = errInjected
			ffs := &faultFS{fault: step}
			p := &Publisher{Now: fixedClock, fs: ffs}
			pub, err := publishCapture(t, p, target, c, id.Recipient().String())
			if step.op != "write" && ffs.calls[step.op] < step.nth {
				t.Fatalf("fault never triggered: %d calls of %s", ffs.calls[step.op], step.op)
			}
			if !causedBy(err, errInjected) {
				t.Fatalf("got %v", err)
			}
			unchanged(t, target, before)
			added := newEntries(t, target, before)
			switch {
			case step.op == "stat" && step.nth == 2, step.op == "mkdir", step.op == "create":
				// Begin failed and withdrew its directory.
				if len(added) != 0 {
					t.Fatalf("run left %v", added)
				}
			case step.op == "syncdir" && step.nth == 2:
				// The rename happened. The generation is complete, but its
				// name may not be durable yet, and the run reports it.
				if pub == nil || len(added) != 1 || added[0] != pub.Name {
					t.Fatalf("published %+v, added %v", pub, added)
				}
			default:
				for _, n := range added {
					if !strings.HasSuffix(n, PartialMarker) {
						t.Errorf("run created %s without the partial marker", n)
					}
				}
			}
		})
	}
}

func TestConcurrentRunsWithIdenticalTimeAndRandomSource(t *testing.T) {
	const runs = 8
	target := t.TempDir()
	id := archivetest.NewKey(t)
	c := captureWith(t, verify.Verified)
	var wg sync.WaitGroup
	errs := make([]error, runs)
	start := make(chan struct{})
	for i := 0; i < runs; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := &Publisher{Now: fixedClock, Rand: sameFirst("0000000f")}
			<-start
			_, errs[i] = publishCapture(t, p, target, c, id.Recipient().String())
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("run %d: %v", i, err)
		}
	}
	got := names(t, target)
	if len(got) != runs {
		t.Fatalf("target holds %d entries: %v", len(got), got)
	}
	for _, n := range got {
		if _, marker, ok := ParseName(n); !ok || marker != "" {
			t.Errorf("name %s", n)
		}
		verifyGeneration(t, filepath.Join(target, n), id)
	}
}

func TestWriteErrorInTheMiddle(t *testing.T) {
	id := archivetest.NewKey(t)
	c := captureWith(t, verify.Verified)
	for _, after := range []int{1, 200, 4096, 20000} {
		for _, injected := range []error{errInjected, syscall.ENOSPC} {
			t.Run(fmt.Sprintf("%v after %d bytes", injected, after), func(t *testing.T) {
				target := populated(t)
				before := snapshot(t, target)
				p := &Publisher{Now: fixedClock, fs: &faultFS{fault: fault{op: "write", writeAfter: after, err: injected}}}
				_, err := publishCapture(t, p, target, c, id.Recipient().String())
				if !causedBy(err, injected) {
					t.Fatalf("got %v", err)
				}
				unchanged(t, target, before)
				// The run aborted and removed its own partial directory.
				if added := newEntries(t, target, before); len(added) != 0 {
					t.Errorf("run left %v", added)
				}
			})
		}
	}
}

func TestCommitRefusesAfterWriteError(t *testing.T) {
	target := t.TempDir()
	c := captureWith(t, verify.Verified)
	p := &Publisher{Now: fixedClock, fs: &faultFS{fault: fault{op: "write", writeAfter: 10, err: syscall.ENOSPC}}}
	g, err := p.Begin(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Writer().Write(make([]byte, 100)); err == nil {
		t.Fatal("write succeeded")
	}
	if _, err := g.Commit(c.verification); !errors.Is(err, ErrWriteFailed) {
		t.Fatalf("commit after write error: %v", err)
	}
	if _, err := g.Commit(c.verification); !errors.Is(err, ErrCommitFailed) {
		t.Fatalf("second commit: %v", err)
	}
	if got := names(t, target); len(got) != 1 || got[0] != g.PartialName() {
		t.Fatalf("target holds %v", got)
	}
	if err := g.Abort(); err != nil {
		t.Fatal(err)
	}
	if got := names(t, target); len(got) != 0 {
		t.Errorf("target holds %v after abort", got)
	}
}

func TestFlushErrorKeepsGenerationPartial(t *testing.T) {
	id := archivetest.NewKey(t)
	c := captureWith(t, verify.Verified)
	for _, step := range []fault{{op: "sync", nth: 1}, {op: "syncdir", nth: 1}} {
		t.Run(step.String(), func(t *testing.T) {
			target := t.TempDir()
			step.err = syscall.EIO
			p := &Publisher{Now: fixedClock, fs: &faultFS{fault: step}}
			_, err := publishCapture(t, p, target, c, id.Recipient().String())
			if !errors.Is(err, syscall.EIO) {
				t.Fatalf("got %v", err)
			}
			got := names(t, target)
			if len(got) != 1 || !strings.HasSuffix(got[0], PartialMarker) {
				t.Fatalf("target holds %v", got)
			}
		})
	}
}

func TestLeftoverFromAnAbortedRun(t *testing.T) {
	target := populated(t)
	before := snapshot(t, target)
	now := time.Date(2026, 1, 3, 2, 30, 0, 0, time.UTC)
	got, err := ListPartial(target, now)
	if err != nil {
		t.Fatal(err)
	}
	want := NamePrefix + "20260103T000000Z-00000003" + PartialMarker
	if len(got) != 1 || got[0].Name != want || got[0].Age != 150*time.Minute {
		t.Fatalf("got %+v", got)
	}
	unchanged(t, target, before)
	if len(snapshot(t, target)) != len(before) {
		t.Error("listing changed the target")
	}

	// Publishing next to the leftover keeps it.
	id := archivetest.NewKey(t)
	if _, err := publishCapture(t, &Publisher{Now: fixedClock}, target, captureWith(t, verify.Verified), id.Recipient().String()); err != nil {
		t.Fatal(err)
	}
	unchanged(t, target, before)
}

func TestMissingTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "missing")
	if _, err := (&Publisher{}).Begin(target); !errors.Is(err, ErrTargetMissing) {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("target exists afterwards: %v", err)
	}
}

func TestTargetIsARegularFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file")
	if err := os.WriteFile(target, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, dir)
	if _, err := (&Publisher{}).Begin(target); !errors.Is(err, ErrTargetNotDirectory) {
		t.Fatalf("got %v", err)
	}
	unchanged(t, dir, before)
	if len(snapshot(t, dir)) != len(before) {
		t.Error("something was created")
	}
}

func TestPermissionsOnLinux(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipped: Windows has no POSIX permissions, and access control lists are not set (non-goal)")
	}
	target := t.TempDir()
	id := archivetest.NewKey(t)
	pub, err := publishCapture(t, &Publisher{}, target, captureWith(t, verify.Verified), id.Recipient().String())
	if err != nil {
		t.Fatal(err)
	}
	di, err := os.Stat(pub.Path)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(pub.Path, ArchiveName))
	if err != nil {
		t.Fatal(err)
	}
	if di.Mode().Perm() != 0o700 || fi.Mode().Perm() != 0o600 {
		t.Errorf("directory %v, file %v", di.Mode().Perm(), fi.Mode().Perm())
	}
}

func TestParseName(t *testing.T) {
	for name, want := range map[string]string{
		baseName("0123abcd"):                    "",
		baseName("0123abcd") + PartialMarker:    PartialMarker,
		baseName("0123abcd") + UnverifiedMarker: UnverifiedMarker,
	} {
		created, marker, ok := ParseName(name)
		if !ok || marker != want || !created.Equal(testNow) {
			t.Errorf("%s: %v %q %v", name, created, marker, ok)
		}
	}
	for _, name := range []string{"notes.txt", baseName("0123ABCD"), baseName("0123abc"), baseName("0123abcd") + ".old"} {
		if _, _, ok := ParseName(name); ok {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestVerdictsMatchPackageVerify(t *testing.T) {
	if VerdictVerified != string(verify.Verified) || VerdictUnverified != string(verify.Unverified) || VerdictFailed != string(verify.Failed) {
		t.Fatal("verdict constants differ from package verify")
	}
}
