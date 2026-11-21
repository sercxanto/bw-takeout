//go:build vaultwarden && linux

package e2e

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

const (
	serverReadyLimit = 30 * time.Second
	serverStopGrace  = 5 * time.Second
)

// vaultServer is one running Vaultwarden instance on loopback with TLS
// (design D3).
type vaultServer struct {
	URL    string // https://localhost:<port>
	CAFile string // the certificate, which is its own CA
	Port   int
	Client *http.Client // trusts the certificate only

	cmd  *exec.Cmd
	done chan struct{}
	log  string // server output, kept in the temporary directory
}

// startServer starts a fresh server in a temporary data directory and
// registers its stop as a cleanup.
func startServer(t *testing.T) *vaultServer {
	t.Helper()
	bin := vaultwardenBinary(t)
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatal(err)
	}
	certFile, keyFile, pool := writeCertificate(t, dir)
	port := freePort(t)

	s := &vaultServer{
		URL:    "https://localhost:" + strconv.Itoa(port),
		CAFile: certFile,
		Port:   port,
		Client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
		},
		done: make(chan struct{}),
		log:  filepath.Join(dir, "server.log"),
	}
	logFile, err := os.OpenFile(s.log, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()

	// The environment of the manual exercise, with the web vault off. The
	// calling environment is not passed on.
	s.cmd = exec.Command(bin)
	s.cmd.Dir = dir
	s.cmd.Env = []string{
		"DATA_FOLDER=" + data,
		"ROCKET_ADDRESS=127.0.0.1",
		"ROCKET_PORT=" + strconv.Itoa(port),
		fmt.Sprintf("ROCKET_TLS={certs=%q,key=%q}", certFile, keyFile),
		"DOMAIN=" + s.URL,
		"SIGNUPS_ALLOWED=true",
		"DISABLE_ICON_DOWNLOAD=true",
		"WEB_VAULT_ENABLED=false",
	}
	s.cmd.Stdout = logFile
	s.cmd.Stderr = logFile
	if err := s.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		s.cmd.Wait()
		close(s.done)
	}()
	t.Cleanup(s.stop)

	if err := s.waitAlive(); err != nil {
		s.stop()
		// No account exists yet, so the start log holds no account data.
		if out, rerr := os.ReadFile(s.log); rerr == nil {
			t.Logf("server output:\n%s", out)
		}
		t.Fatal(err)
	}
	return s
}

func (s *vaultServer) waitAlive() error {
	deadline := time.Now().Add(serverReadyLimit)
	for time.Now().Before(deadline) {
		select {
		case <-s.done:
			return fmt.Errorf("the server ended before it was ready: %v", s.cmd.ProcessState)
		default:
		}
		resp, err := s.Client.Get(s.URL + "/alive")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("the server was not ready within %v", serverReadyLimit)
}

// stop ends the server: SIGTERM, then SIGKILL after the grace period. It
// can be called more than once.
func (s *vaultServer) stop() {
	select {
	case <-s.done:
		return
	default:
	}
	s.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-s.done:
	case <-time.After(serverStopGrace):
		s.cmd.Process.Kill()
		<-s.done
	}
}

// freePort returns a port that was free a moment ago. The listener is
// closed right before the start, because DOMAIN must name the port.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

// writeCertificate creates a self-signed ECDSA P-256 certificate for
// localhost and 127.0.0.1, valid for one day, and writes it and its key
// with mode 0600.
func writeCertificate(t *testing.T, dir string) (certFile, keyFile string, pool *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}), 0o600); err != nil {
		t.Fatal(err)
	}
	pool = x509.NewCertPool()
	pool.AddCert(cert)
	return certFile, keyFile, pool
}

func TestServerStartsAndStops(t *testing.T) {
	s := startServer(t)
	resp, err := s.Client.Get(s.URL + "/alive")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/alive: status %d", resp.StatusCode)
	}

	pid := s.cmd.Process.Pid
	start := time.Now()
	s.stop()
	t.Logf("stopped in %v", time.Since(start).Round(time.Millisecond))
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("the server process %d is still there: %v", pid, err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(s.Port))
	if err != nil {
		t.Fatalf("the port is not free after the stop: %v", err)
	}
	l.Close()
}
