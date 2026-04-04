package proxy

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// Store handles proxy_requests persistence.
type Store struct {
	db *sqlx.DB
}

// NewStore creates a new proxy store and ensures the schema exists.
func NewStore(db *sqlx.DB) *Store {
	s := &Store{db: db}
	s.migrate()
	return s
}

func (s *Store) migrate() {
	schema := `
	CREATE TABLE IF NOT EXISTS proxy_requests (
		id                 INTEGER PRIMARY KEY AUTOINCREMENT,
		request_id         TEXT NOT NULL UNIQUE,
		session_id         TEXT NOT NULL,
		agent_name         TEXT,
		agent_type         TEXT,
		board_name         TEXT,

		provider           TEXT NOT NULL,
		model_requested    TEXT NOT NULL,
		model_used         TEXT NOT NULL,
		is_streaming       INTEGER NOT NULL DEFAULT 0,

		input_tokens       INTEGER NOT NULL DEFAULT 0,
		output_tokens      INTEGER NOT NULL DEFAULT 0,
		cache_read_tokens  INTEGER NOT NULL DEFAULT 0,
		cache_write_tokens INTEGER NOT NULL DEFAULT 0,
		total_tokens       INTEGER NOT NULL DEFAULT 0,
		input_cost_usd     REAL NOT NULL DEFAULT 0,
		output_cost_usd    REAL NOT NULL DEFAULT 0,
		cache_read_cost_usd REAL NOT NULL DEFAULT 0,
		cache_write_cost_usd REAL NOT NULL DEFAULT 0,
		pricing_input_per_mtok REAL NOT NULL DEFAULT 0,
		pricing_output_per_mtok REAL NOT NULL DEFAULT 0,
		pricing_cache_read_per_mtok REAL NOT NULL DEFAULT 0,
		pricing_cache_write_per_mtok REAL NOT NULL DEFAULT 0,

		cost_usd           REAL NOT NULL DEFAULT 0,

		started_at         TEXT NOT NULL,
		completed_at       TEXT,
		latency_ms         INTEGER,

		status             TEXT NOT NULL DEFAULT 'pending',
		error_message      TEXT,
		http_status        INTEGER,

		cache_hit          INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_proxy_requests_session ON proxy_requests(session_id);
	CREATE INDEX IF NOT EXISTS idx_proxy_requests_started ON proxy_requests(started_at);
	CREATE INDEX IF NOT EXISTS idx_proxy_requests_model ON proxy_requests(model_used);
	`
	s.db.MustExec(schema)

	for _, stmt := range []string{
		"ALTER TABLE proxy_requests ADD COLUMN input_cost_usd REAL NOT NULL DEFAULT 0",
		"ALTER TABLE proxy_requests ADD COLUMN output_cost_usd REAL NOT NULL DEFAULT 0",
		"ALTER TABLE proxy_requests ADD COLUMN cache_read_cost_usd REAL NOT NULL DEFAULT 0",
		"ALTER TABLE proxy_requests ADD COLUMN cache_write_cost_usd REAL NOT NULL DEFAULT 0",
		"ALTER TABLE proxy_requests ADD COLUMN pricing_input_per_mtok REAL NOT NULL DEFAULT 0",
		"ALTER TABLE proxy_requests ADD COLUMN pricing_output_per_mtok REAL NOT NULL DEFAULT 0",
		"ALTER TABLE proxy_requests ADD COLUMN pricing_cache_read_per_mtok REAL NOT NULL DEFAULT 0",
		"ALTER TABLE proxy_requests ADD COLUMN pricing_cache_write_per_mtok REAL NOT NULL DEFAULT 0",
	} {
		s.db.Exec(stmt) // ignore duplicate-column errors
	}
}

// ProxyRequest represents a row in proxy_requests.
type ProxyRequest struct {
	ID                       int64   `db:"id" json:"id"`
	RequestID                string  `db:"request_id" json:"request_id"`
	SessionID                string  `db:"session_id" json:"session_id"`
	AgentName                *string `db:"agent_name" json:"agent_name"`
	AgentType                *string `db:"agent_type" json:"agent_type"`
	BoardName                *string `db:"board_name" json:"board_name"`
	Provider                 string  `db:"provider" json:"provider"`
	ModelRequested           string  `db:"model_requested" json:"model_requested"`
	ModelUsed                string  `db:"model_used" json:"model_used"`
	IsStreaming              int     `db:"is_streaming" json:"is_streaming"`
	InputTokens              int     `db:"input_tokens" json:"input_tokens"`
	OutputTokens             int     `db:"output_tokens" json:"output_tokens"`
	CacheReadTokens          int     `db:"cache_read_tokens" json:"cache_read_tokens"`
	CacheWriteTokens         int     `db:"cache_write_tokens" json:"cache_write_tokens"`
	TotalTokens              int     `db:"total_tokens" json:"total_tokens"`
	InputCostUSD             float64 `db:"input_cost_usd" json:"input_cost_usd"`
	OutputCostUSD            float64 `db:"output_cost_usd" json:"output_cost_usd"`
	CacheReadCostUSD         float64 `db:"cache_read_cost_usd" json:"cache_read_cost_usd"`
	CacheWriteCostUSD        float64 `db:"cache_write_cost_usd" json:"cache_write_cost_usd"`
	PricingInputPerMTok      float64 `db:"pricing_input_per_mtok" json:"pricing_input_per_mtok"`
	PricingOutputPerMTok     float64 `db:"pricing_output_per_mtok" json:"pricing_output_per_mtok"`
	PricingCacheReadPerMTok  float64 `db:"pricing_cache_read_per_mtok" json:"pricing_cache_read_per_mtok"`
	PricingCacheWritePerMTok float64 `db:"pricing_cache_write_per_mtok" json:"pricing_cache_write_per_mtok"`
	CostUSD                  float64 `db:"cost_usd" json:"cost_usd"`
	StartedAt                string  `db:"started_at" json:"started_at"`
	CompletedAt              *string `db:"completed_at" json:"completed_at"`
	LatencyMS                *int    `db:"latency_ms" json:"latency_ms"`
	Status                   string  `db:"status" json:"status"`
	ErrorMessage             *string `db:"error_message" json:"error_message"`
	HTTPStatus               *int    `db:"http_status" json:"http_status"`
	CacheHit                 int     `db:"cache_hit" json:"cache_hit"`
}

// CreateRequest inserts a new pending proxy request.
func (s *Store) CreateRequest(ctx context.Context, reqID, sessionID string, provider Provider, model string, streaming bool) error {
	isStream := 0
	if streaming {
		isStream = 1
	}

	var agentName, agentType, boardName *string
	var meta struct {
		AgentName *string `db:"agent_name"`
		AgentType *string `db:"agent_type"`
		BoardName *string `db:"board_name"`
	}
	if err := s.db.GetContext(ctx, &meta,
		`SELECT agent_name, agent_type, board_name FROM live_sessions WHERE session_id = ?`,
		sessionID); err == nil {
		agentName = meta.AgentName
		agentType = meta.AgentType
		boardName = meta.BoardName
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO proxy_requests
		 (request_id, session_id, agent_name, agent_type, board_name, provider, model_requested, model_used, is_streaming, started_at, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending')`,
		reqID, sessionID, agentName, agentType, boardName, string(provider), model, model, isStream, time.Now().UTC().Format(time.RFC3339))
	return err
}

// CompleteRequest updates a request with final usage and status.
func (s *Store) CompleteRequest(ctx context.Context, reqID string, usage TokenUsage, breakdown CostBreakdown, httpStatus int, status string, errMsg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var errPtr *string
	if errMsg != "" {
		errPtr = &errMsg
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE proxy_requests SET
			input_tokens = ?, output_tokens = ?, cache_read_tokens = ?, cache_write_tokens = ?,
			total_tokens = ?, input_cost_usd = ?, output_cost_usd = ?, cache_read_cost_usd = ?, cache_write_cost_usd = ?,
			pricing_input_per_mtok = ?, pricing_output_per_mtok = ?, pricing_cache_read_per_mtok = ?, pricing_cache_write_per_mtok = ?,
			cost_usd = ?, completed_at = ?,
			latency_ms = (strftime('%s', ?) - strftime('%s', started_at)) * 1000,
			status = ?, error_message = ?, http_status = ?
		 WHERE request_id = ?`,
		usage.InputTokens, usage.OutputTokens, usage.CacheReadTokens, usage.CacheWriteTokens,
		usage.InputTokens+usage.OutputTokens+usage.CacheReadTokens+usage.CacheWriteTokens,
		breakdown.InputCostUSD, breakdown.OutputCostUSD, breakdown.CacheReadCostUSD, breakdown.CacheWriteCostUSD,
		breakdown.Pricing.InputPerMTok, breakdown.Pricing.OutputPerMTok, breakdown.Pricing.CacheReadPerMTok, breakdown.Pricing.CacheWritePerMTok,
		breakdown.TotalCostUSD, now, now, status, errPtr, httpStatus, reqID)
	return err
}

// ListRequests returns recent proxy requests with optional filtering.
func (s *Store) ListRequests(ctx context.Context, sessionID string, limit, offset int) ([]ProxyRequest, int, error) {
	where := "1=1"
	args := []any{}
	if sessionID != "" {
		where += " AND session_id = ?"
		args = append(args, sessionID)
	}

	var total int
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	err := s.db.GetContext(ctx, &total, "SELECT COUNT(*) FROM proxy_requests WHERE "+where, countArgs...)
	if err != nil {
		return nil, 0, err
	}

	query := "SELECT * FROM proxy_requests WHERE " + where + " ORDER BY started_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	var rows []ProxyRequest
	err = s.db.SelectContext(ctx, &rows, query, args...)
	return rows, total, err
}

// Stats returns aggregated cost stats for a time period.
type StatsResult struct {
	TotalRequests     int     `json:"total_requests"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
	TotalInputTokens  int     `json:"total_input_tokens"`
	TotalOutputTokens int     `json:"total_output_tokens"`
}

// ModelStats holds per-model aggregates.
type ModelStats struct {
	Model    string  `db:"model_used" json:"model"`
	Requests int     `db:"requests" json:"requests"`
	CostUSD  float64 `db:"cost_usd" json:"cost_usd"`
}

// GetStats returns aggregate stats, optionally filtered.
func (s *Store) GetStats(ctx context.Context, since string, sessionID string) (StatsResult, []ModelStats, error) {
	where := "started_at >= ?"
	args := []any{since}
	if sessionID != "" {
		where += " AND session_id = ?"
		args = append(args, sessionID)
	}

	var stats StatsResult
	err := s.db.GetContext(ctx, &stats,
		`SELECT COUNT(*) as total_requests, COALESCE(SUM(cost_usd),0) as total_cost_usd,
		 COALESCE(SUM(input_tokens),0) as total_input_tokens, COALESCE(SUM(output_tokens),0) as total_output_tokens
		 FROM proxy_requests WHERE `+where, args...)
	if err != nil {
		return stats, nil, err
	}

	var byModel []ModelStats
	modelArgs := make([]any, len(args))
	copy(modelArgs, args)
	err = s.db.SelectContext(ctx, &byModel,
		`SELECT model_used, COUNT(*) as requests, COALESCE(SUM(cost_usd),0) as cost_usd
		 FROM proxy_requests WHERE `+where+` GROUP BY model_used ORDER BY cost_usd DESC`, modelArgs...)
	return stats, byModel, err
}

// SessionCost returns cost summary for a single session.
func (s *Store) SessionCost(ctx context.Context, sessionID string) (StatsResult, []ModelStats, error) {
	return s.GetStats(ctx, "1970-01-01", sessionID)
}

// GetRequestByID returns a single proxy request by its UUID request_id.
func (s *Store) GetRequestByID(ctx context.Context, requestID string) (*ProxyRequest, error) {
	var req ProxyRequest
	err := s.db.GetContext(ctx, &req, "SELECT * FROM proxy_requests WHERE request_id = ?", requestID)
	if err != nil {
		return nil, err
	}
	return &req, nil
}

// AgentStats holds per-agent (session) aggregates.
type AgentStats struct {
	SessionID string  `db:"session_id" json:"session_id"`
	AgentName *string `db:"agent_name" json:"agent_name"`
	Requests  int     `db:"requests" json:"requests"`
	CostUSD   float64 `db:"cost_usd" json:"cost_usd"`
}

// TaskRunCost holds proxy cost totals for a scheduled/task run.
type TaskRunCost struct {
	RunID             int64   `db:"run_id" json:"run_id"`
	SessionID         *string `db:"session_id" json:"session_id,omitempty"`
	TotalRequests     int     `db:"total_requests" json:"total_requests"`
	TotalCostUSD      float64 `db:"total_cost_usd" json:"total_cost_usd"`
	TotalInputTokens  int     `db:"total_input_tokens" json:"total_input_tokens"`
	TotalOutputTokens int     `db:"total_output_tokens" json:"total_output_tokens"`
}

// GetStatsByAgent returns per-agent cost breakdown for a time period.
func (s *Store) GetStatsByAgent(ctx context.Context, since string) ([]AgentStats, error) {
	var byAgent []AgentStats
	err := s.db.SelectContext(ctx, &byAgent,
		`SELECT session_id, agent_name, COUNT(*) as requests, COALESCE(SUM(cost_usd),0) as cost_usd
		 FROM proxy_requests WHERE started_at >= ?
		 GROUP BY session_id ORDER BY cost_usd DESC`, since)
	if err != nil {
		return nil, err
	}
	if byAgent == nil {
		byAgent = []AgentStats{}
	}
	return byAgent, nil
}

// GetTaskRunCost returns proxy cost totals for one scheduled/task run.
func (s *Store) GetTaskRunCost(ctx context.Context, runID int64) (TaskRunCost, error) {
	var row TaskRunCost
	err := s.db.GetContext(ctx, &row,
		`SELECT sr.id AS run_id, sr.session_id AS session_id,
		        COUNT(pr.id) AS total_requests,
		        COALESCE(SUM(pr.cost_usd), 0) AS total_cost_usd,
		        COALESCE(SUM(pr.input_tokens), 0) AS total_input_tokens,
		        COALESCE(SUM(pr.output_tokens), 0) AS total_output_tokens
		   FROM scheduled_runs sr
		   LEFT JOIN proxy_requests pr ON pr.session_id = sr.session_id
		  WHERE sr.id = ?
		  GROUP BY sr.id, sr.session_id`,
		runID)
	return row, err
}

// GetTaskRunCosts returns proxy cost totals for a batch of runs.
func (s *Store) GetTaskRunCosts(ctx context.Context, runIDs []int64) (map[int64]TaskRunCost, error) {
	result := make(map[int64]TaskRunCost, len(runIDs))
	if len(runIDs) == 0 {
		return result, nil
	}

	query, args, err := sqlx.In(
		`SELECT sr.id AS run_id, sr.session_id AS session_id,
		        COUNT(pr.id) AS total_requests,
		        COALESCE(SUM(pr.cost_usd), 0) AS total_cost_usd,
		        COALESCE(SUM(pr.input_tokens), 0) AS total_input_tokens,
		        COALESCE(SUM(pr.output_tokens), 0) AS total_output_tokens
		   FROM scheduled_runs sr
		   LEFT JOIN proxy_requests pr ON pr.session_id = sr.session_id
		  WHERE sr.id IN (?)
		  GROUP BY sr.id, sr.session_id`,
		runIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("build task run cost query: %w", err)
	}
	query = s.db.Rebind(query)

	var rows []TaskRunCost
	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.RunID] = row
	}
	return result, nil
}
