package executil

import (
	"os/exec"
	"runtime"
)

// OpenBrowser opens the given URL in the user's default browser.
// It is best-effort; errors are silently ignored.
func OpenBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}
