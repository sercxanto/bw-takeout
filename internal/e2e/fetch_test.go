//go:build vaultwarden && linux

package e2e

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"testing"
	"time"
)

// The pinned Vaultwarden 1.37.4 image (design D2). The index digest fixes
// both manifests and every blob. The table repeats what the index implies,
// so that a reader sees which binary runs and a mismatch is named.
const (
	vaultwardenRepository = "vaultwarden/server"
	vaultwardenIndex      = "sha256:19ea2b669945d842dadc3d500f2d6a795866a003330f59a47b9634658bae98bb"
	vaultwardenVersion    = "1.37.4"
)

// serverPin is the manifest digest and the SHA-256 of the server binary
// for one architecture.
type serverPin struct {
	manifest string
	binary   string
}

var vaultwardenPins = map[string]serverPin{
	"arm64": {
		manifest: "sha256:c82bc74bb0a88821fd3fbce48e7867e8aca4e492e512970a309d02a629ab91d4",
		binary:   "ae658947f85964e934d7673d9cab4c0edbce8e6167422d6ff3b10008e125421d",
	},
	"amd64": {
		manifest: "sha256:7d90670a4e2531e752f014e58f7283b9d197847b6f073bf84e99598f49d80ffd",
		binary:   "392bc9afa7de1821e372a6d93369c87c1070644c68e26057017a2bf1b9cac3be",
	},
}

const (
	mediaIndex    = "application/vnd.oci.image.index.v1+json"
	mediaManifest = "application/vnd.oci.image.manifest.v1+json"
	// maxBlob bounds every download and the unpacked binary.
	maxBlob = 256 << 20
)

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// fetcher gets the server binary from a registry by digest and keeps every
// blob in a cache keyed by its digest. Every use checks the cached file
// again, so a damaged cache is fetched anew, never trusted.
type fetcher struct {
	registry   string // base URL of the registry API, without /v2
	auth       string // URL of the token service
	service    string // service parameter of the token request
	repository string
	index      string
	pins       map[string]serverPin
	cache      string
	client     *http.Client

	token    string
	requests int // requests sent, for the tests of the cache
}

// dockerHub is the fetcher of the pinned image from Docker Hub with the
// cache under .local/vaultwarden-cache/.
func dockerHub(cache string) *fetcher {
	return &fetcher{
		registry:   "https://registry-1.docker.io",
		auth:       "https://auth.docker.io/token",
		service:    "registry.docker.io",
		repository: vaultwardenRepository,
		index:      vaultwardenIndex,
		pins:       vaultwardenPins,
		cache:      cache,
		client:     &http.Client{Timeout: 5 * time.Minute},
	}
}

var (
	serverOnce sync.Once
	serverPath string
	serverErr  error
)

// vaultwardenBinary returns the path of the checked server binary for the
// running architecture. It fetches once per test run.
func vaultwardenBinary(t testing.TB) string {
	t.Helper()
	serverOnce.Do(func() {
		cache := filepath.Join(repoRoot(t), ".local", "vaultwarden-cache")
		serverPath, serverErr = dockerHub(cache).server(runtime.GOARCH)
	})
	if serverErr != nil {
		t.Fatalf("Vaultwarden %s: %v", vaultwardenVersion, serverErr)
	}
	return serverPath
}

// server returns the path of the server binary for linux/arch in the
// cache, after its SHA-256 matched the table.
func (f *fetcher) server(arch string) (string, error) {
	pin, ok := f.pins[arch]
	if !ok {
		return "", fmt.Errorf("no pinned server for linux/%s, only for %s", arch, pinnedArchitectures(f.pins))
	}
	if !digestPattern.MatchString("sha256:" + pin.binary) {
		return "", fmt.Errorf("linux/%s: malformed binary hash in the table", arch)
	}
	binPath := f.cachePath("sha256:" + pin.binary)
	if ok, err := checkFile(binPath, pin.binary); err != nil {
		return "", err
	} else if ok {
		return binPath, nil
	}

	raw, err := f.blob("manifests", f.index, mediaIndex)
	if err != nil {
		return "", err
	}
	manifest, err := selectManifest(raw, arch)
	if err != nil {
		return "", err
	}
	if manifest != pin.manifest {
		return "", fmt.Errorf("linux/%s: the index names the manifest %s, the table %s", arch, manifest, pin.manifest)
	}
	raw, err = f.blob("manifests", manifest, mediaManifest)
	if err != nil {
		return "", err
	}
	var m struct {
		Layers []struct {
			Digest string `json:"digest"`
		} `json:"layers"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", fmt.Errorf("linux/%s manifest: %w", arch, err)
	}
	// The topmost layer that holds the file wins, as in the image.
	for i := len(m.Layers) - 1; i >= 0; i-- {
		layer, err := f.blob("blobs", m.Layers[i].Digest, "*/*")
		if err != nil {
			return "", err
		}
		bin, err := unpackServer(layer)
		if err != nil {
			return "", fmt.Errorf("layer %s: %w", m.Layers[i].Digest, err)
		}
		if bin == nil {
			continue
		}
		if got := sha256Hex(bin); got != pin.binary {
			return "", fmt.Errorf("linux/%s: the server binary has the SHA-256 %s, the table %s", arch, got, pin.binary)
		}
		if err := writeCache(binPath, bin, 0o700); err != nil {
			return "", err
		}
		return binPath, nil
	}
	return "", fmt.Errorf("linux/%s: no layer holds the file vaultwarden", arch)
}

func pinnedArchitectures(pins map[string]serverPin) string {
	var names []string
	for _, a := range []string{"amd64", "arm64"} {
		if _, ok := pins[a]; ok {
			names = append(names, a)
		}
	}
	return fmt.Sprint(names)
}

// selectManifest returns the digest of the one manifest for linux/arch in
// an image index.
func selectManifest(raw []byte, arch string) (string, error) {
	var index struct {
		Manifests []struct {
			Digest   string `json:"digest"`
			Platform struct {
				OS           string `json:"os"`
				Architecture string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(raw, &index); err != nil {
		return "", fmt.Errorf("image index: %w", err)
	}
	var found []string
	for _, m := range index.Manifests {
		if m.Platform.OS == "linux" && m.Platform.Architecture == arch {
			found = append(found, m.Digest)
		}
	}
	if len(found) != 1 {
		return "", fmt.Errorf("linux/%s: the index names %d manifests, want 1", arch, len(found))
	}
	return found[0], nil
}

// blob returns the content of one digest, from the cache when the cached
// file matches, otherwise from the registry. kind is manifests or blobs.
func (f *fetcher) blob(kind, digest, accept string) ([]byte, error) {
	if !digestPattern.MatchString(digest) {
		return nil, fmt.Errorf("malformed digest %q", digest)
	}
	want := digest[len("sha256:"):]
	p := f.cachePath(digest)
	if data, err := os.ReadFile(p); err == nil && sha256Hex(data) == want {
		return data, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	data, err := f.get(kind, digest, accept)
	if err != nil {
		return nil, err
	}
	if got := sha256Hex(data); got != want {
		return nil, fmt.Errorf("%s %s: the content has the SHA-256 %s", kind, digest, got)
	}
	if err := writeCache(p, data, 0o600); err != nil {
		return nil, err
	}
	return data, nil
}

func (f *fetcher) get(kind, digest, accept string) ([]byte, error) {
	if f.token == "" {
		if err := f.getToken(); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequest(http.MethodGet, f.registry+"/v2/"+f.repository+"/"+kind+"/"+digest, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("Accept", accept)
	f.requests++
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", kind, digest, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s %s: status %d", kind, digest, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBlob+1))
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", kind, digest, err)
	}
	if len(data) > maxBlob {
		return nil, fmt.Errorf("%s %s: larger than %d bytes", kind, digest, maxBlob)
	}
	return data, nil
}

// getToken gets an anonymous pull token. The token is not logged.
func (f *fetcher) getToken() error {
	q := url.Values{"service": {f.service}, "scope": {"repository:" + f.repository + ":pull"}}
	f.requests++
	resp, err := f.client.Get(f.auth + "?" + q.Encode())
	if err != nil {
		return fmt.Errorf("pull token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pull token: status %d", resp.StatusCode)
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return fmt.Errorf("pull token: %w", err)
	}
	if body.Token == "" {
		return errors.New("pull token: empty")
	}
	f.token = body.Token
	return nil
}

func (f *fetcher) cachePath(digest string) string {
	return filepath.Join(f.cache, "sha256", digest[len("sha256:"):])
}

// unpackServer returns the content of the file vaultwarden at the root of
// a gzip-compressed layer, or nil when the layer does not hold it. Nothing
// else is unpacked.
func unpackServer(layer []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(layer))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if h.Name != "vaultwarden" && h.Name != "./vaultwarden" {
			continue
		}
		if h.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("vaultwarden is not a regular file")
		}
		data, err := io.ReadAll(io.LimitReader(tr, maxBlob+1))
		if err != nil {
			return nil, err
		}
		if len(data) > maxBlob {
			return nil, fmt.Errorf("vaultwarden is larger than %d bytes", maxBlob)
		}
		return data, nil
	}
}

// checkFile reports whether the file at p has the SHA-256 want. A missing
// file is false without an error. A file with another hash is removed, so
// that it is fetched anew.
func checkFile(p, want string) (bool, error) {
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if sha256Hex(data) == want {
		return true, nil
	}
	return false, os.Remove(p)
}

// writeCache writes data to p through a temporary file and a rename, so
// that an interrupted write leaves no file under the final name.
func writeCache(p string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".partial-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
