package ptymanager

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cdknorow/coral/internal/tmux"
)

// newTestTmuxBackend creates a TmuxBackend with an isolated tmux socket
// and logDir. Cleans up the tmux server on test completion.
func newTestTmuxBackend(t *testing.T) *TmuxBackend {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	sock := filepath.Join(t.TempDir(), "test.sock")
	client := tmux.NewClient()
	client.SocketPath = sock
	client.FallbackToDefault = false

	logDir := filepath.Join(t.TempDir(), "logs")
	os.MkdirAll(logDir, 0755)

	backend := NewTmuxBackend(client, logDir)

	t.Cleanup(func() {
		exec.Command("tmux", "-S", sock, "kill-server").Run()
	})

	return backend
}

func TestTmuxBackend_SpawnAndKill(t *testing.T) {
	b := newTestTmuxBackend(t)

	err := b.Spawn("test-agent", "claude", t.TempDir(), "sid-001", "sleep 60", 80, 24)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	if !b.IsRunning("test-agent") {
		t.Error("expected session to be running after spawn")
	}

	err = b.Kill("test-agent")
	if err != nil {
		t.Fatalf("Kill failed: %v", err)
	}

	if b.IsRunning("test-agent") {
		t.Error("expected session to not be running after kill")
	}
}

func TestTmuxBackend_SpawnCreatesLogFile(t *testing.T) {
	b := newTestTmuxBackend(t)

	err := b.Spawn("log-test", "claude", t.TempDir(), "sid-log", "sleep 60", 80, 24)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer b.Kill("log-test")

	logPath := b.LogPath("log-test")
	if logPath == "" {
		t.Fatal("expected non-empty log path")
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Errorf("log file does not exist: %v", err)
	}
}

func TestTmuxBackend_CaptureContent(t *testing.T) {
	b := newTestTmuxBackend(t)

	err := b.Spawn("capture-test", "claude", t.TempDir(), "sid-cap", "sh -c 'echo MARKER_CAPTURE_42'", 80, 24)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer b.Kill("capture-test")

	// Wait for command output
	var content string
	for i := 0; i < 20; i++ {
		time.Sleep(200 * time.Millisecond)
		content, _ = b.CaptureContent("capture-test")
		if strings.Contains(content, "MARKER_CAPTURE_42") {
			return // success
		}
	}
	t.Errorf("expected capture to contain MARKER_CAPTURE_42, got: %q", content)
}

func TestTmuxBackend_SendInput(t *testing.T) {
	b := newTestTmuxBackend(t)

	err := b.Spawn("input-test", "claude", t.TempDir(), "sid-inp", "cat", 80, 24)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer b.Kill("input-test")

	time.Sleep(500 * time.Millisecond)

	err = b.SendInput("input-test", []byte("echo SEND_TEST_MARKER\n"))
	if err != nil {
		t.Fatalf("SendInput failed: %v", err)
	}

	var content string
	for i := 0; i < 20; i++ {
		time.Sleep(200 * time.Millisecond)
		content, _ = b.CaptureContent("input-test")
		if strings.Contains(content, "SEND_TEST_MARKER") {
			return
		}
	}
	t.Errorf("expected capture to contain SEND_TEST_MARKER, got: %q", content)
}

func TestTmuxBackend_ListSessions(t *testing.T) {
	b := newTestTmuxBackend(t)

	// Use UUID-formatted session IDs so ParseSessionName can extract them
	sidA := "aaaaaaaa-1111-2222-3333-444444444444"
	sidB := "bbbbbbbb-1111-2222-3333-444444444444"

	err := b.Spawn("agent1", "claude", t.TempDir(), sidA, "sleep 60", 80, 24)
	if err != nil {
		t.Fatalf("Spawn 1 failed: %v", err)
	}
	defer b.Kill("agent1")

	err = b.Spawn("agent2", "gemini", t.TempDir(), sidB, "sleep 60", 80, 24)
	if err != nil {
		t.Fatalf("Spawn 2 failed: %v", err)
	}
	defer b.Kill("agent2")

	time.Sleep(500 * time.Millisecond)

	sessions := b.ListSessions()
	if len(sessions) < 2 {
		t.Fatalf("expected at least 2 sessions, got %d", len(sessions))
	}

	found := map[string]bool{}
	for _, s := range sessions {
		found[s.SessionID] = true
	}
	if !found[sidA] {
		t.Errorf("expected to find session %s", sidA)
	}
	if !found[sidB] {
		t.Errorf("expected to find session %s", sidB)
	}
}

func TestTmuxBackend_Restart(t *testing.T) {
	b := newTestTmuxBackend(t)

	err := b.Spawn("restart-test", "claude", t.TempDir(), "sid-rst", "sleep 60", 80, 24)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer b.Kill("restart-test")

	time.Sleep(500 * time.Millisecond)

	err = b.Restart("restart-test", "echo RESTARTED_MARKER")
	if err != nil {
		t.Fatalf("Restart failed: %v", err)
	}

	var content string
	for i := 0; i < 20; i++ {
		time.Sleep(200 * time.Millisecond)
		content, _ = b.CaptureContent("restart-test")
		if strings.Contains(content, "RESTARTED_MARKER") {
			return
		}
	}
	t.Errorf("expected capture to contain RESTARTED_MARKER after restart, got: %q", content)
}

func TestTmuxBackend_Resize(t *testing.T) {
	b := newTestTmuxBackend(t)

	err := b.Spawn("resize-test", "claude", t.TempDir(), "sid-rsz", "sleep 60", 80, 24)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer b.Kill("resize-test")

	time.Sleep(300 * time.Millisecond)

	err = b.Resize("resize-test", 120, 40)
	if err != nil {
		t.Errorf("Resize failed: %v", err)
	}
}

func TestTmuxBackend_LogPath(t *testing.T) {
	b := newTestTmuxBackend(t)

	// Unknown session returns empty
	if got := b.LogPath("nonexistent"); got != "" {
		t.Errorf("expected empty log path for unknown session, got %q", got)
	}

	err := b.Spawn("logpath-test", "claude", t.TempDir(), "sid-lp", "sleep 60", 80, 24)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer b.Kill("logpath-test")

	logPath := b.LogPath("logpath-test")
	if !strings.Contains(logPath, "claude_coral_sid-lp.log") {
		t.Errorf("unexpected log path: %q", logPath)
	}
}

func TestTmuxBackend_KillNotFound(t *testing.T) {
	b := newTestTmuxBackend(t)

	// Killing a nonexistent session should not panic (may return error)
	_ = b.Kill("nonexistent-session")
}

func TestTmuxBackend_CaptureNotFound(t *testing.T) {
	b := newTestTmuxBackend(t)

	// Capturing a nonexistent session may return error or empty content
	// depending on tmux behavior — just verify no panic
	content, _ := b.CaptureContent("nonexistent-session")
	if content != "" {
		t.Errorf("expected empty content for nonexistent session, got %q", content)
	}
}

func TestTmuxBackend_Subscribe(t *testing.T) {
	b := newTestTmuxBackend(t)

	ch, err := b.Subscribe("any", "ws-1")
	if err != nil {
		t.Errorf("Subscribe should not error: %v", err)
	}
	if ch != nil {
		t.Error("expected nil channel (polling mode)")
	}
}

func TestTmuxBackend_Close(t *testing.T) {
	b := newTestTmuxBackend(t)

	err := b.Spawn("close-test", "claude", t.TempDir(), "sid-cls", "sleep 60", 80, 24)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	// Close should be a no-op (not kill sessions)
	if err := b.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Session should still be running in tmux after Close
	if !b.IsRunning("close-test") {
		t.Error("expected session to still be running after Close")
	}

	b.Kill("close-test")
}

func TestTmuxBackend_ConcurrentSpawnKill(t *testing.T) {
	b := newTestTmuxBackend(t)

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := strings.Replace("conc-IDX", "IDX", string(rune('a'+idx)), 1)
			sid := strings.Replace("sid-conc-IDX", "IDX", string(rune('0'+idx)), 1)
			b.Spawn(name, "claude", t.TempDir(), sid, "sleep 60", 80, 24)
		}(i)
	}
	wg.Wait()

	time.Sleep(500 * time.Millisecond)

	var wg2 sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg2.Add(1)
		go func(idx int) {
			defer wg2.Done()
			name := strings.Replace("conc-IDX", "IDX", string(rune('a'+idx)), 1)
			b.Kill(name)
		}(i)
	}
	wg2.Wait()
}
