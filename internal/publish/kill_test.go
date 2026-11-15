package publish

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"bw-takeout/internal/archive"
	"bw-takeout/internal/archive/archivetest"
	"bw-takeout/internal/capture/capturetest"
)

const (
	helperTargetEnv    = "BW_TAKEOUT_KILL_HELPER_TARGET"
	helperRecipientEnv = "BW_TAKEOUT_KILL_HELPER_RECIPIENT"
)

// TestKillHelper is not a test of its own. TestProcessKilledWhileWriting
// starts the test binary with it, and it then writes part of an archive into
// a new generation, reports "ready" and waits to be killed (design D12).
func TestKillHelper(t *testing.T) {
	target := os.Getenv(helperTargetEnv)
	if target == "" {
		t.Skip("helper process for TestProcessKilledWhileWriting, runs only when started by it")
	}
	g, err := (&Publisher{}).Begin(target)
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	f := capturetest.Intact(t)
	w, err := archive.NewWriter(g.Writer(), []string{os.Getenv(helperRecipientEnv)}, archivetest.Created, archivetest.Documents(t, f))
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	// The attachment is larger than one age chunk of 64 KiB, so ciphertext
	// beyond the header reaches the file before the process is killed.
	large := archive.Attachment{
		ItemID: capturetest.ItemNote, AttachmentID: "large000000000000000000000000001",
		Content: []byte(strings.Repeat("synthetic ", 20000)),
	}
	if err := w.AddAttachment(large); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	fmt.Println("ready")
	time.Sleep(time.Minute)
	fmt.Println("error: not killed")
	os.Exit(1)
}

func TestProcessKilledWhileWriting(t *testing.T) {
	target := populated(t)
	before := snapshot(t, target)
	id := archivetest.NewKey(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestKillHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), helperTargetEnv+"="+target, helperRecipientEnv+"="+id.Recipient().String())
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ready := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			line := sc.Text()
			if line == "ready" || strings.HasPrefix(line, "error:") {
				ready <- line
				return
			}
		}
		ready <- "helper ended without reporting"
	}()
	select {
	case line := <-ready:
		if line != "ready" {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			t.Fatalf("helper: %s", line)
		}
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("helper did not report ready")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()

	unchanged(t, target, before)
	added := newEntries(t, target, before)
	if len(added) != 1 || !strings.HasSuffix(added[0], PartialMarker) {
		t.Fatalf("killed run left %v", added)
	}
	info, err := os.Stat(target + string(os.PathSeparator) + added[0] + string(os.PathSeparator) + ArchiveName)
	if err != nil || info.Size() <= 64*1024 {
		t.Fatalf("partial archive holds no full chunk: %v", err)
	}
	t.Logf("killed run left %s with %d bytes of ciphertext", added[0], info.Size())
	leftovers, err := ListPartial(target, testNow)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range leftovers {
		found = found || l.Name == added[0]
	}
	if !found {
		t.Errorf("leftover not listed: %+v", leftovers)
	}
}
