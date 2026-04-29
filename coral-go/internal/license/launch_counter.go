package license

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// LaunchCounter tracks how many times Coral has been launched.
// Used to decide when to show the "please consider buying a license" nag screen.
type LaunchCounter struct {
	path string
}

func NewLaunchCounter(coralDir string) *LaunchCounter {
	return &LaunchCounter{path: filepath.Join(coralDir, ".launch-count")}
}

// Increment bumps the counter and returns the new value.
func (lc *LaunchCounter) Increment() int {
	count := lc.read() + 1
	os.WriteFile(lc.path, []byte(strconv.Itoa(count)), 0644)
	return count
}

// IsNagLaunch returns true if the activation nag screen should be shown.
// Shows on launch 1, 4, 7, 10, ... (every 3rd launch, starting from the first).
func (lc *LaunchCounter) IsNagLaunch() bool {
	return lc.read()%3 == 1
}

func (lc *LaunchCounter) read() int {
	data, err := os.ReadFile(lc.path)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return n
}
