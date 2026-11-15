package profile

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 15, 17, 0, 0, 0, time.UTC)

func fixedClock() time.Time { return testNow }

// sequence returns a random source that yields the given values in order
// and then random values.
func sequence(values ...string) io.Reader {
	var b []byte
	for _, v := range values {
		d, err := hex.DecodeString(v)
		if err != nil || len(d) != 4 {
			panic("bad value")
		}
		b = append(b, d...)
	}
	return io.MultiReader(bytes.NewReader(b), rand.Reader)
}

func nameAt(created time.Time, random string) string {
	return NamePrefix + created.Format(timeLayout) + "-" + random
}

// snapshot maps every path under root to "dir" or the SHA-256 of the file.
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

func unchanged(t *testing.T, root string, before map[string]string) {
	t.Helper()
	after := snapshot(t, root)
	for path, sum := range before {
		if after[path] != sum {
			t.Errorf("%s changed or vanished", path)
		}
	}
}

func mustCreate(t *testing.T, c *Creator) *Profile {
	t.Helper()
	p, err := c.Create()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// leftoverAt writes a directory that looks like a profile of a killed run,
// with a data file, and returns its name.
func leftoverAt(t *testing.T, loc string, created time.Time, random string) string {
	t.Helper()
	name := nameAt(created, random)
	data := filepath.Join(loc, name, DataDirName)
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(loc, name, WorkDirName), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "data.json"), []byte(`{"leftover":"`+random+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return name
}

func gone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s still exists: %v", path, err)
	}
}

func TestTwoRunsInTheSameSecond(t *testing.T) {
	loc := t.TempDir()
	// Real random source: two runs at the same clock value.
	a := mustCreate(t, &Creator{Location: loc, Now: fixedClock})
	b := mustCreate(t, &Creator{Location: loc, Now: fixedClock})
	if a.Name == b.Name || a.Path == b.Path {
		t.Fatalf("same profile %s", a.Name)
	}
	// Both runs draw the same first random part: the second gets another.
	loc2 := t.TempDir()
	c := mustCreate(t, &Creator{Location: loc2, Now: fixedClock, Rand: sequence("0123abcd")})
	d := mustCreate(t, &Creator{Location: loc2, Now: fixedClock, Rand: sequence("0123abcd", "4567ef01")})
	if c.Name != nameAt(testNow, "0123abcd") || d.Name != nameAt(testNow, "4567ef01") {
		t.Fatalf("got %s and %s", c.Name, d.Name)
	}
	for _, p := range []*Profile{a, b, c, d} {
		if fi, err := os.Stat(p.Path); err != nil || !fi.IsDir() {
			t.Errorf("%s: %v", p.Path, err)
		}
	}
}

func TestChosenNameAlreadyExists(t *testing.T) {
	loc := t.TempDir()
	existing := leftoverAt(t, loc, testNow, "0123abcd")
	before := snapshot(t, loc)
	p := mustCreate(t, &Creator{Location: loc, Now: fixedClock, Rand: sequence("0123abcd", "0123abcd", "89abcdef")})
	if p.Name != nameAt(testNow, "89abcdef") {
		t.Fatalf("got %s", p.Name)
	}
	if p.Name == existing {
		t.Fatal("existing directory used")
	}
	unchanged(t, loc, before)
	if entries, err := os.ReadDir(filepath.Join(loc, existing, WorkDirName)); err != nil || len(entries) != 0 {
		t.Errorf("existing directory changed: %v %v", entries, err)
	}
}

func TestNoFreeName(t *testing.T) {
	loc := t.TempDir()
	leftoverAt(t, loc, testNow, "0123abcd")
	values := make([]string, maxAttempts)
	for i := range values {
		values[i] = "0123abcd"
	}
	before := snapshot(t, loc)
	if _, err := (&Creator{Location: loc, Now: fixedClock, Rand: sequence(values...)}).Create(); !errors.Is(err, ErrNoFreeName) {
		t.Fatalf("got %v", err)
	}
	unchanged(t, loc, before)
	if len(snapshot(t, loc)) != len(before) {
		t.Error("something was created")
	}
}

func TestProfileLocationIsMissing(t *testing.T) {
	loc := filepath.Join(t.TempDir(), "cache", "bw-takeout", "profiles")
	p := mustCreate(t, &Creator{Location: loc, Now: fixedClock})
	if fi, err := os.Stat(loc); err != nil || !fi.IsDir() {
		t.Fatalf("location: %v", err)
	}
	if filepath.Dir(p.Path) != loc {
		t.Errorf("profile %s is not inside %s", p.Path, loc)
	}
}

func TestNewProfile(t *testing.T) {
	loc := t.TempDir()
	p := mustCreate(t, &Creator{Location: loc, Now: fixedClock})
	if !filepath.IsAbs(p.Path) || p.Path != filepath.Join(loc, p.Name) {
		t.Errorf("path %s", p.Path)
	}
	if created, ok := ParseName(p.Name); !ok || !created.Equal(testNow) || !p.Created.Equal(testNow) {
		t.Errorf("name %s, created %v", p.Name, p.Created)
	}
	if p.DataDir == p.WorkDir {
		t.Fatal("data and working directory are the same")
	}
	for _, dir := range []string{p.DataDir, p.WorkDir} {
		if filepath.Dir(dir) != p.Path {
			t.Errorf("%s is not inside the profile", dir)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("%s is not empty", dir)
		}
	}
	entries, err := os.ReadDir(p.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("profile holds %d entries", len(entries))
	}
}

func TestCreationTimeIsUTC(t *testing.T) {
	local := time.FixedZone("UTC+5", 5*3600)
	p := mustCreate(t, &Creator{Location: t.TempDir(), Now: func() time.Time { return testNow.In(local) }})
	if !strings.Contains(p.Name, "20261115T170000Z") {
		t.Errorf("name %s", p.Name)
	}
}

func TestParseName(t *testing.T) {
	if created, ok := ParseName(nameAt(testNow, "0123abcd")); !ok || !created.Equal(testNow) {
		t.Errorf("valid name refused")
	}
	for _, name := range []string{
		"notes",
		nameAt(testNow, "0123ABCD"),
		nameAt(testNow, "0123abc"),
		nameAt(testNow, "0123abcd") + ".old",
		"profile-20261315T170000Z-0123abcd",
		"x" + nameAt(testNow, "0123abcd"),
	} {
		if _, ok := ParseName(name); ok {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestDefaultLocation(t *testing.T) {
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Skipf("skipped: the system reports no user cache directory: %v", err)
	}
	loc, err := DefaultLocation()
	if err != nil {
		t.Fatal(err)
	}
	if loc != filepath.Join(cache, "bw-takeout", "profiles") {
		t.Errorf("got %s", loc)
	}
}

func TestPermissionsOnLinuxAndMacOS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipped: Windows has no POSIX permissions, and access control lists are not set (non-goal)")
	}
	loc := filepath.Join(t.TempDir(), "profiles")
	p := mustCreate(t, &Creator{Location: loc, Now: fixedClock})
	for _, dir := range []string{loc, p.Path, p.DataDir, p.WorkDir} {
		fi, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o700 {
			t.Errorf("%s has mode %v", dir, fi.Mode().Perm())
		}
	}
}

// fillProfile writes what bw leaves in a profile after a run.
func fillProfile(t *testing.T, p *Profile) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(p.DataDir, "data.json"), []byte(`{"tokens":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(p.DataDir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.DataDir, "sub", "cache"), []byte("cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.WorkDir, "stray"), []byte("stray"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTeardownAfterASuccessfulRun(t *testing.T) {
	loc := t.TempDir()
	p := mustCreate(t, &Creator{Location: loc, Now: fixedClock})
	fillProfile(t, p)
	calls := 0
	logoutErr, removeErr := p.Teardown(func() error {
		calls++
		// The logout runs while the profile still exists.
		if _, err := os.Stat(filepath.Join(p.DataDir, "data.json")); err != nil {
			t.Errorf("profile gone before the logout: %v", err)
		}
		return nil
	})
	if logoutErr != nil || removeErr != nil {
		t.Fatalf("logout %v, removal %v", logoutErr, removeErr)
	}
	if calls != 1 {
		t.Errorf("logout called %d times", calls)
	}
	gone(t, p.Path)
	if _, err := os.Stat(loc); err != nil {
		t.Errorf("location removed: %v", err)
	}
}

func TestLogoutFails(t *testing.T) {
	p := mustCreate(t, &Creator{Location: t.TempDir(), Now: fixedClock})
	fillProfile(t, p)
	errLogout := errors.New("logout failed")
	logoutErr, removeErr := p.Teardown(func() error { return errLogout })
	if !errors.Is(logoutErr, errLogout) {
		t.Errorf("logout error %v", logoutErr)
	}
	if removeErr != nil {
		t.Errorf("removal %v", removeErr)
	}
	gone(t, p.Path)
}

func TestRemovalFails(t *testing.T) {
	errRemove := errors.New("injected removal error")
	var removed string
	c := &Creator{Location: t.TempDir(), Now: fixedClock, removeAll: func(path string) error {
		removed = path
		return errRemove
	}}
	p := mustCreate(t, c)
	errLogout := errors.New("logout failed")
	logoutErr, removeErr := p.Teardown(func() error { return errLogout })
	if !errors.Is(logoutErr, errLogout) {
		t.Errorf("logout error %v", logoutErr)
	}
	if removed != p.Path {
		t.Errorf("removal of %q", removed)
	}
	var re *RemoveError
	if !errors.As(removeErr, &re) || re.Path != p.Path || !errors.Is(removeErr, errRemove) {
		t.Fatalf("removal error %v", removeErr)
	}
	if !strings.Contains(removeErr.Error(), p.Path) {
		t.Errorf("error text %q does not name %s", removeErr, p.Path)
	}
}

func TestTeardownWithoutLogin(t *testing.T) {
	p := mustCreate(t, &Creator{Location: t.TempDir(), Now: fixedClock})
	fillProfile(t, p)
	logoutErr, removeErr := p.Teardown(nil)
	if logoutErr != nil || removeErr != nil {
		t.Fatalf("logout %v, removal %v", logoutErr, removeErr)
	}
	gone(t, p.Path)
}

func TestLeftoverFromAKilledRun(t *testing.T) {
	loc := t.TempDir()
	created := testNow.Add(-26 * time.Hour)
	name := leftoverAt(t, loc, created, "deadbeef")
	before := snapshot(t, loc)
	got, err := (&Creator{Location: loc, Now: fixedClock}).Leftovers(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != name || got[0].Age != 26*time.Hour || got[0].Path != filepath.Join(loc, name) {
		t.Fatalf("got %+v", got)
	}
	unchanged(t, loc, before)
	if len(snapshot(t, loc)) != len(before) {
		t.Error("something was created")
	}
}

func TestOwnProfile(t *testing.T) {
	loc := t.TempDir()
	other := leftoverAt(t, loc, testNow.Add(-time.Hour), "deadbeef")
	c := &Creator{Location: loc, Now: fixedClock}
	own := mustCreate(t, c)
	before := snapshot(t, loc)
	got, err := c.Leftovers(own)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != other {
		t.Fatalf("got %+v", got)
	}
	unchanged(t, loc, before)
	// Without the own profile, both are listed in name order.
	got, err = c.Leftovers(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != other || got[1].Name != own.Name {
		t.Fatalf("got %+v", got)
	}
}

func TestUnrelatedEntriesInTheProfileLocation(t *testing.T) {
	loc := t.TempDir()
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(loc, name), []byte("unrelated "+name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mkdir := func(name string) {
		if err := os.MkdirAll(filepath.Join(loc, name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(loc, name, "file"), []byte("unrelated "+name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("notes.txt")
	write(nameAt(testNow, "0123abcd")) // a file with a profile name
	mkdir("notes")
	// Another random part than the file above, because the file systems of
	// macOS and Windows do not tell names apart by case.
	mkdir(nameAt(testNow, "4567ABCD"))
	mkdir(nameAt(testNow, "0123abcd") + ".old")
	before := snapshot(t, loc)
	got, err := (&Creator{Location: loc, Now: fixedClock}).Leftovers(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
	unchanged(t, loc, before)
	if len(snapshot(t, loc)) != len(before) {
		t.Error("something was created")
	}
}

func TestLeftoversWithoutLocation(t *testing.T) {
	loc := filepath.Join(t.TempDir(), "profiles")
	got, err := (&Creator{Location: loc, Now: fixedClock}).Leftovers(nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v, %v", got, err)
	}
	gone(t, loc)
}
