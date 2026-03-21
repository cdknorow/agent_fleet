package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GeminiAgent implements the Agent interface for Gemini.
type GeminiAgent struct{}

func (a *GeminiAgent) AgentType() string    { return "gemini" }
func (a *GeminiAgent) SupportsResume() bool { return false }

func (a *GeminiAgent) HistoryBasePath() string {
	if v := os.Getenv("GEMINI_TMP_DIR"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gemini", "tmp")
}

func (a *GeminiAgent) HistoryGlobPattern() string { return "session-*.json" }

func (a *GeminiAgent) BuildLaunchCommand(params LaunchParams) string {
	var cmd string
	if params.ProtocolPath != "" {
		if _, err := os.Stat(params.ProtocolPath); err == nil {
			cmd = fmt.Sprintf(`GEMINI_SYSTEM_MD="%s" gemini`, params.ProtocolPath)
		} else {
			cmd = "gemini"
		}
	} else {
		cmd = "gemini"
	}
	if len(params.Flags) > 0 {
		cmd += " " + strings.Join(params.Flags, " ")
	}
	return cmd
}

func (a *GeminiAgent) PrepareResume(sessionID, workingDir string) {
	// Gemini does not support resume
}
