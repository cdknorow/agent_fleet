package background

import "context"

// AgentRuntime abstracts how agents are spawned, monitored, and communicated with.
// TmuxRuntime implements this via tmux sessions; PTYRuntime uses native PTY sessions.
// This allows background services to work on both Unix (tmux) and Windows (ConPTY).
type AgentRuntime interface {
	// SpawnAgent creates a new agent session with the given name, working directory,
	// log file path, and command to execute.
	SpawnAgent(ctx context.Context, name, workDir, logFile, command string) error

	// SendInput sends text input to a running agent session (for nudges, prompts).
	SendInput(ctx context.Context, name, text string) error

	// KillAgent terminates an agent session by name.
	KillAgent(ctx context.Context, name string) error

	// IsAlive checks if an agent session is still running.
	IsAlive(ctx context.Context, name string) bool

	// ListAgents discovers all running agent sessions.
	ListAgents(ctx context.Context) ([]AgentInfo, error)
}
