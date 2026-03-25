package background

import (
	"context"
	"log/slog"
	"time"

	"github.com/cdknorow/coral/internal/store"
)

// SessionReconciler periodically checks live sessions against running
// processes and marks crashed agents as sleeping. This catches agents that
// die between server restarts — without it, they stay "live" in the DB
// forever with no running process.
type SessionReconciler struct {
	sessionStore *store.SessionStore
	runtime      AgentRuntime
	interval     time.Duration
	logger       *slog.Logger
}

// NewSessionReconciler creates a new SessionReconciler.
func NewSessionReconciler(ss *store.SessionStore, rt AgentRuntime, interval time.Duration) *SessionReconciler {
	return &SessionReconciler{
		sessionStore: ss,
		runtime:      rt,
		interval:     interval,
		logger:       slog.Default().With("service", "session_reconciler"),
	}
}

// Run starts the reconciliation loop.
func (r *SessionReconciler) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			r.reconcileOnce(ctx)
		}
	}
}

func (r *SessionReconciler) reconcileOnce(ctx context.Context) {
	liveSessions, err := r.sessionStore.GetAllLiveSessions(ctx)
	if err != nil {
		r.logger.Error("failed to read live sessions", "error", err)
		return
	}

	agents, err := r.runtime.ListAgents(ctx)
	if err != nil {
		r.logger.Error("failed to list running agents", "error", err)
		return
	}
	alive := make(map[string]bool, len(agents))
	for _, a := range agents {
		alive[a.SessionID] = true
	}

	for _, ls := range liveSessions {
		if ls.IsSleeping == 1 {
			continue
		}
		if alive[ls.SessionID] {
			continue
		}
		if err := r.sessionStore.SetSessionSleeping(ctx, ls.SessionID, true); err != nil {
			r.logger.Error("failed to mark crashed agent as sleeping",
				"session_id", ls.SessionID, "agent_name", ls.AgentName, "error", err)
			continue
		}
		r.logger.Warn("detected crashed agent, marking as sleeping",
			"session_id", ls.SessionID, "agent_name", ls.AgentName)
	}
}
