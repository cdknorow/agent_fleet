package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ShellType identifies the type of shell being used for agent sessions.
type ShellType string

const (
	ShellBash       ShellType = "bash"
	ShellZsh        ShellType = "zsh"
	ShellPowerShell ShellType = "powershell"
	ShellCmd        ShellType = "cmd"
)

// DetectShell determines the shell type from CORAL_SHELL env var,
// falling back to platform defaults.
func DetectShell() ShellType {
	if env := os.Getenv("CORAL_SHELL"); env != "" {
		return classifyShell(env)
	}

	if runtime.GOOS == "windows" {
		return ShellPowerShell
	}

	// Unix: check $SHELL
	if sh := os.Getenv("SHELL"); sh != "" {
		return classifyShell(sh)
	}
	return ShellBash
}

// classifyShell maps a shell path or name to a ShellType.
func classifyShell(shell string) ShellType {
	base := strings.ToLower(filepath.Base(shell))
	// Strip .exe suffix for Windows
	base = strings.TrimSuffix(base, ".exe")

	switch {
	case base == "pwsh" || base == "powershell":
		return ShellPowerShell
	case base == "cmd":
		return ShellCmd
	case base == "zsh":
		return ShellZsh
	default:
		// bash, sh, git-bash, wsl, etc.
		return ShellBash
	}
}

// FormatPromptFileArg returns shell-appropriate syntax for reading a prompt
// file and passing its content as a CLI argument.
func FormatPromptFileArg(promptFile string) string {
	shell := DetectShell()
	switch shell {
	case ShellPowerShell:
		return fmt.Sprintf("$(Get-Content -Raw '%s')", promptFile)
	case ShellCmd:
		// cmd.exe doesn't support inline file content substitution.
		// Use a workaround: pipe file content. But since this is a positional
		// arg, we use PowerShell-style (cmd users should use PowerShell for agents).
		return fmt.Sprintf("$(Get-Content -Raw '%s')", promptFile)
	default:
		// bash, zsh, sh
		return fmt.Sprintf("\"$(cat '%s')\"", promptFile)
	}
}
