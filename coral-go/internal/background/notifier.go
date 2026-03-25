package background

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/cdknorow/coral/internal/board"
)

// BoardNotifier nudges agents when they have unread board messages.
type BoardNotifier struct {
	boardStore *board.Store
	runtime    AgentRuntime
	interval   time.Duration
	logger     *slog.Logger
	discoverFn func(ctx context.Context) ([]AgentInfo, error)
	isPausedFn func(project string) bool
	notified   map[string]int // session_id -> unread count at last notification
}

// NewBoardNotifier creates a new BoardNotifier.
func NewBoardNotifier(boardStore *board.Store, runtime AgentRuntime, interval time.Duration) *BoardNotifier {
	return &BoardNotifier{
		boardStore: boardStore,
		runtime:    runtime,
		interval:   interval,
		logger:     slog.Default().With("service", "board_notifier"),
		notified:   make(map[string]int),
	}
}

// SetDiscoverFn sets a custom agent discovery function.
func (n *BoardNotifier) SetDiscoverFn(fn func(ctx context.Context) ([]AgentInfo, error)) {
	n.discoverFn = fn
}

// SetIsPausedFn sets a function to check if a board project is paused.
func (n *BoardNotifier) SetIsPausedFn(fn func(project string) bool) {
	n.isPausedFn = fn
}

// Run starts the notification loop.
func (n *BoardNotifier) Run(ctx context.Context) error {
	ticker := time.NewTicker(n.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := n.RunOnce(ctx); err != nil {
				n.logger.Error("notification error", "error", err)
			}
		}
	}
}

// RunOnce performs a single notification pass.
func (n *BoardNotifier) RunOnce(ctx context.Context) error {
	agents, err := n.discoverFn(ctx)
	if err != nil {
		return err
	}

	liveBoardIDs := make(map[string]bool)

	for _, agent := range agents {
		if agent.SessionID == "" {
			continue
		}

		// Board uses tmux session name as subscriber ID: {type}-{uuid}
		boardSID := fmt.Sprintf("%s-%s", agent.AgentType, agent.SessionID)
		liveBoardIDs[boardSID] = true

		sub, err := n.boardStore.GetSubscription(ctx, boardSID)
		if err != nil || sub == nil {
			continue
		}
		// Skip remote subscribers
		if sub.OriginServer != nil && *sub.OriginServer != "" {
			continue
		}
		// Skip agents on paused/sleeping boards
		if n.isPausedFn != nil && sub.Project != "" && n.isPausedFn(sub.Project) {
			continue
		}

		unread, err := n.boardStore.CheckUnread(ctx, sub.Project, boardSID)
		if err != nil {
			continue
		}

		if unread == 0 {
			delete(n.notified, boardSID)
			continue
		}

		if n.notified[boardSID] == unread {
			continue
		}

		// Send nudge
		plural := "s"
		if unread == 1 {
			plural = ""
		}
		nudge := fmt.Sprintf("You have %d unread message%s on the message board. Run 'coral-board read' to see them.", unread, plural)
		sessionName := fmt.Sprintf("%s-%s", agent.AgentType, agent.SessionID)
		err = n.runtime.SendInput(ctx, sessionName, nudge)
		if err != nil {
			n.logger.Warn("failed to nudge agent", "agent", agent.AgentName, "error", err)
			continue
		}

		n.notified[boardSID] = unread
	}

	// Clean up stale entries
	for sid := range n.notified {
		if !liveBoardIDs[sid] {
			delete(n.notified, sid)
		}
	}

	return nil
}
