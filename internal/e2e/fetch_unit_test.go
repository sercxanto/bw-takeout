//go:build vaultwarden && linux

package e2e

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeRegistry serves one synthetic image index with an arm64 and an amd64
// manifest. Each manifest has a layer without the server and, on top, a
// layer with it.
type fakeRegistry struct {
	srv      *httptest.Server
	blobs    map[string][]byte // by digest
	pins     map[string]serverPin
	binaries map[string][]byte // by architecture
	index    string
	requests atomic.Int64
	// corrupt names a digest whose content is served with a changed byte.
	corrupt string
}

func digestOf(data []byte) string { return "sha256:" + sha256Hex(data) }

func layer(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newFakeRegistry(t *testing.T) *fakeRegistry {
	t.Helper()
	r := &fakeRegistry{blobs: map[string][]byte{}, pins: map[string]serverPin{}, binaries: map[string][]byte{}}
	add := func(data []byte) string {
		d := digestOf(data)
		r.blobs[d] = data
		return d
	}
	type desc struct {
		MediaType string            `json:"mediaType"`
		Digest    string            `json:"digest"`
		Platform  map[string]string `json:"platform,omitempty"`
	}
	var manifests []desc
	for _, arch := range []string{"amd64", "arm64"} {
		bin := make([]byte, 4096)
		rand.Read(bin)
		r.binaries[arch] = bin
		base := add(layer(t, map[string][]byte{"etc/os-release": []byte(arch)}))
		top := add(layer(t, map[string][]byte{"vaultwarden": bin, "web-vault/index.html": []byte("x")}))
		m, err := json.Marshal(map[string]any{
			"mediaType": mediaManifest,
			"layers":    []desc{{Digest: base}, {Digest: top}},
		})
		if err != nil {
			t.Fatal(err)
		}
		md := add(m)
		r.pins[arch] = serverPin{manifest: md, binary: sha256Hex(bin)}
		manifests = append(manifests, desc{MediaType: mediaManifest, Digest: md, Platform: map[string]string{"os": "linux", "architecture": arch}})
	}
	manifests = append(manifests, desc{MediaType: mediaManifest, Digest: add([]byte("{}")), Platform: map[string]string{"os": "unknown", "architecture": "unknown"}})
	idx, err := json.Marshal(map[string]any{"mediaType": mediaIndex, "manifests": manifests})
	if err != nil {
		t.Fatal(err)
	}
	r.index = add(idx)

	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.requests.Add(1)
		if req.URL.Path == "/token" {
			w.Write([]byte(`{"token":"anonymous"}`))
			return
		}
		if req.Header.Get("Authorization") != "Bearer anonymous" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		prefix := "/v2/" + vaultwardenRepository + "/"
		rest, ok := strings.CutPrefix(req.URL.Path, prefix)
		if !ok {
			http.NotFound(w, req)
			return
		}
		_, d, _ := strings.Cut(rest, "/")
		data, ok := r.blobs[d]
		if !ok {
			http.NotFound(w, req)
			return
		}
		if d == r.corrupt {
			data = append([]byte{}, data...)
			data[len(data)/2] ^= 0xff
		}
		w.Write(data)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *fakeRegistry) fetcher(cache string) *fetcher {
	return &fetcher{
		registry:   r.srv.URL,
		auth:       r.srv.URL + "/token",
		service:    "test",
		repository: vaultwardenRepository,
		index:      r.index,
		pins:       r.pins,
		cache:      cache,
		client:     r.srv.Client(),
	}
}

func wantBinary(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("the unpacked binary differs from the one in the layer")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("mode %v, want 0700", info.Mode().Perm())
	}
}

func TestFetchUnpacksServerOfArchitecture(t *testing.T) {
	r := newFakeRegistry(t)
	for _, arch := range []string{"arm64", "amd64"} {
		p, err := r.fetcher(t.TempDir()).server(arch)
		if err != nil {
			t.Fatalf("%s: %v", arch, err)
		}
		wantBinary(t, p, r.binaries[arch])
	}
}

func TestFetchBlobWithWrongDigestFails(t *testing.T) {
	r := newFakeRegistry(t)
	var m struct {
		Layers []struct{ Digest string } `json:"layers"`
	}
	if err := json.Unmarshal(r.blobs[r.pins["arm64"].manifest], &m); err != nil {
		t.Fatal(err)
	}
	for name, d := range map[string]string{"index": r.index, "manifest": r.pins["arm64"].manifest, "layer": m.Layers[1].Digest} {
		r.corrupt = d
		cache := t.TempDir()
		_, err := r.fetcher(cache).server("arm64")
		if err == nil || !strings.Contains(err.Error(), d) || !strings.Contains(err.Error(), "SHA-256") {
			t.Fatalf("%s: err = %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(cache, "sha256", d[len("sha256:"):])); !os.IsNotExist(err) {
			t.Fatalf("%s: the corrupt blob was cached", name)
		}
	}
}

func TestFetchManifestDifferentFromTableFails(t *testing.T) {
	r := newFakeRegistry(t)
	f := r.fetcher(t.TempDir())
	f.pins = map[string]serverPin{"arm64": {manifest: r.pins["amd64"].manifest, binary: r.pins["arm64"].binary}}
	_, err := f.server("arm64")
	if err == nil || !strings.Contains(err.Error(), "linux/arm64") || !strings.Contains(err.Error(), "manifest") {
		t.Fatalf("err = %v", err)
	}
}

func TestFetchBinaryDifferentFromTableFails(t *testing.T) {
	r := newFakeRegistry(t)
	f := r.fetcher(t.TempDir())
	f.pins = map[string]serverPin{"arm64": {manifest: r.pins["arm64"].manifest, binary: r.pins["amd64"].binary}}
	_, err := f.server("arm64")
	if err == nil || !strings.Contains(err.Error(), "linux/arm64") || !strings.Contains(err.Error(), "server binary") {
		t.Fatalf("err = %v", err)
	}
}

func TestFetchUnknownArchitectureFails(t *testing.T) {
	r := newFakeRegistry(t)
	_, err := r.fetcher(t.TempDir()).server("riscv64")
	if err == nil || !strings.Contains(err.Error(), "linux/riscv64") {
		t.Fatalf("err = %v", err)
	}
	if n := r.requests.Load(); n != 0 {
		t.Fatalf("%d requests for an unknown architecture", n)
	}
}

func TestFetchDamagedCacheIsFetchedAnew(t *testing.T) {
	r := newFakeRegistry(t)
	cache := t.TempDir()
	p, err := r.fetcher(cache).server("arm64")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(cache, "sha256"))
	if err != nil {
		t.Fatal(err)
	}
	// Damage every cached file: the binary, the layers, the manifest and
	// the index.
	for _, e := range entries {
		if err := os.WriteFile(filepath.Join(cache, "sha256", e.Name()), []byte("damaged"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before := r.requests.Load()
	p2, err := r.fetcher(cache).server("arm64")
	if err != nil {
		t.Fatal(err)
	}
	if r.requests.Load() == before {
		t.Fatal("a damaged cache was used without a request")
	}
	if p2 != p {
		t.Fatalf("path %s, want %s", p2, p)
	}
	wantBinary(t, p2, r.binaries["arm64"])
}

func TestFetchValidCacheNeedsNoRequest(t *testing.T) {
	r := newFakeRegistry(t)
	cache := t.TempDir()
	if _, err := r.fetcher(cache).server("arm64"); err != nil {
		t.Fatal(err)
	}
	before := r.requests.Load()
	f := r.fetcher(cache)
	p, err := f.server("arm64")
	if err != nil {
		t.Fatal(err)
	}
	if n := r.requests.Load() - before; n != 0 || f.requests != 0 {
		t.Fatalf("%d requests with a valid cache", n)
	}
	wantBinary(t, p, r.binaries["arm64"])
}

// TestVaultwardenFromDockerHub fetches the pinned image, or takes it from
// the cache, and checks the binary against the table of design D2.
func TestVaultwardenFromDockerHub(t *testing.T) {
	p := vaultwardenBinary(t)
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := sha256Hex(data), vaultwardenPins[runtime.GOARCH].binary; got != want {
		t.Fatalf("SHA-256 %s, want %s", got, want)
	}
	t.Logf("linux/%s server binary %s", runtime.GOARCH, sha256Hex(data))
}
