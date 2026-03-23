package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CodexAgent implements the Agent interface for OpenAI Codex CLI.
type CodexAgent struct{}

func (a *CodexAgent) AgentType() string    { return "codex" }
func (a *CodexAgent) SupportsResume() bool { return true }

func (a *CodexAgent) HistoryBasePath() string {
	if v := os.Getenv("CODEX_HOME"); v != "" {
		return filepath.Join(v, "sessions")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "sessions")
}

func (a *CodexAgent) HistoryGlobPattern() string { return "rollout-*.jsonl" }

func (a *CodexAgent) BuildLaunchCommand(params LaunchParams) string {
	var parts []string
	parts = append(parts, "codex")

	if params.ResumeSessionID != "" {
		parts = append(parts, "--resume", params.ResumeSessionID)
	}

	if len(params.Flags) > 0 {
		parts = append(parts, params.Flags...)
	}

	// Inject system prompt: combine protocol file + board system prompt
	var sysParts []string
	if params.ProtocolPath != "" {
		if content, err := os.ReadFile(params.ProtocolPath); err == nil {
			sysParts = append(sysParts, string(content))
		}
	}
	boardSysPrompt := BuildBoardSystemPrompt(params.BoardName, params.Role, params.Prompt, params.PromptOverrides, params.BoardType)
	if boardSysPrompt != "" {
		sysParts = append(sysParts, boardSysPrompt)
	}
	if len(sysParts) > 0 {
		sysFile := filepath.Join(os.TempDir(), fmt.Sprintf("coral_codex_sys_%s.txt", params.SessionID))
		os.WriteFile(sysFile, []byte(strings.Join(sysParts, "\n\n")), 0600)
		parts = append(parts, fmt.Sprintf(`--system-prompt "$(cat '%s')"`, sysFile))
	}

	// Build action prompt using shared helper
	cliPrompt := BuildBoardActionPrompt(params.BoardName, params.Role, params.Prompt, params.PromptOverrides, params.BoardType)
	if cliPrompt != "" {
		parts = append(parts, fmt.Sprintf(`"%s"`, strings.ReplaceAll(cliPrompt, `"`, `\"`)))
	}

	return strings.Join(parts, " ")
}

func (a *CodexAgent) PrepareResume(sessionID, workingDir string) {
	// Codex handles resume natively via --resume flag; no file preparation needed.
}
