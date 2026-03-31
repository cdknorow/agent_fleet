package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/cdknorow/coral/internal/background"
	"github.com/cdknorow/coral/internal/config"
	"github.com/cdknorow/coral/internal/store"
)

var workflowNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// WorkflowHandler handles workflow API endpoints.
type WorkflowHandler struct {
	ws     *store.WorkflowStore
	cfg    *config.Config
	runner *background.WorkflowRunner
}

// SetWorkflowRunner injects the workflow runner for triggering and killing runs.
func (h *WorkflowHandler) SetWorkflowRunner(wr *background.WorkflowRunner) {
	h.runner = wr
}

// NewWorkflowHandler creates a new WorkflowHandler.
func NewWorkflowHandler(db *store.DB, cfg *config.Config) *WorkflowHandler {
	return &WorkflowHandler{
		ws:  store.NewWorkflowStore(db),
		cfg: cfg,
	}
}

// ── Workflow CRUD ─────────────────────────────────────────────────────

// ListWorkflows returns all workflows with last_run summary.
// GET /api/workflows
func (h *WorkflowHandler) ListWorkflows(w http.ResponseWriter, r *http.Request) {
	workflows, err := h.ws.ListWorkflows(r.Context())
	if err != nil {
		errInternalServer(w, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workflows": emptyIfNil(workflows)})
}

// GetWorkflow returns a single workflow by ID.
// GET /api/workflows/{workflowID}
func (h *WorkflowHandler) GetWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID, err := strconv.ParseInt(chi.URLParam(r, "workflowID"), 10, 64)
	if err != nil {
		errBadRequest(w, "invalid workflow ID")
		return
	}
	wf, err := h.ws.GetWorkflow(r.Context(), workflowID)
	if err != nil {
		errInternalServer(w, err.Error())
		return
	}
	if wf == nil {
		errNotFound(w, "workflow not found")
		return
	}
	writeJSON(w, http.StatusOK, wf)
}

// GetWorkflowByName returns a single workflow by name.
// GET /api/workflows/by-name/{name}
func (h *WorkflowHandler) GetWorkflowByName(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	wf, err := h.ws.GetWorkflowByName(r.Context(), name)
	if err != nil {
		errInternalServer(w, err.Error())
		return
	}
	if wf == nil {
		errNotFound(w, "workflow not found")
		return
	}
	writeJSON(w, http.StatusOK, wf)
}

// CreateWorkflow creates a new workflow.
// POST /api/workflows
func (h *WorkflowHandler) CreateWorkflow(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name            string              `json:"name"`
		Description     string              `json:"description"`
		Steps           []json.RawMessage   `json:"steps"`
		DefaultAgent    json.RawMessage     `json:"default_agent"`
		RepoPath        string              `json:"repo_path"`
		BaseBranch      string              `json:"base_branch"`
		MaxDurationS    *int                `json:"max_duration_s"`
		CleanupWorktree *int                `json:"cleanup_worktree"`
		Enabled         *int                `json:"enabled"`
	}
	if err := decodeJSON(r, &body); err != nil {
		errBadRequest(w, "invalid JSON")
		return
	}

	// Validate name
	if body.Name == "" {
		errBadRequest(w, "name is required")
		return
	}
	if !workflowNameRe.MatchString(body.Name) {
		errBadRequest(w, "name contains invalid characters")
		return
	}

	// Check name uniqueness
	existing, _ := h.ws.GetWorkflowByName(r.Context(), body.Name)
	if existing != nil {
		errConflict(w, "workflow name already exists")
		return
	}

	// Validate steps
	if len(body.Steps) == 0 {
		errBadRequest(w, "at least one step is required")
		return
	}
	if len(body.Steps) > 20 {
		errBadRequest(w, "maximum 20 steps allowed")
		return
	}

	// Validate step contents
	if errMsg := validateSteps(body.Steps, body.DefaultAgent); errMsg != "" {
		errBadRequest(w, errMsg)
		return
	}

	// Validate and canonicalize repo_path if provided
	if body.RepoPath != "" {
		cleaned := filepath.Clean(body.RepoPath)
		resolved, err := filepath.EvalSymlinks(cleaned)
		if err != nil {
			errBadRequest(w, "repo_path does not exist")
			return
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() {
			errBadRequest(w, "repo_path does not exist")
			return
		}
		body.RepoPath = resolved
	}

	// Validate max_duration_s
	if body.MaxDurationS != nil && (*body.MaxDurationS <= 0 || *body.MaxDurationS > 86400) {
		errBadRequest(w, "invalid max_duration")
		return
	}

	// Marshal steps and default_agent to JSON strings
	stepsJSON, err := store.StepsJSONFromRaw(body.Steps)
	if err != nil {
		errBadRequest(w, "invalid steps")
		return
	}
	var defaultAgentJSON string
	if body.DefaultAgent != nil {
		defaultAgentJSON = string(body.DefaultAgent)
	}

	wf := &store.Workflow{
		Name:             body.Name,
		Description:      body.Description,
		StepsJSON:        stepsJSON,
		DefaultAgentJSON: defaultAgentJSON,
		RepoPath:         body.RepoPath,
		BaseBranch:       body.BaseBranch,
		MaxDurationS:     intPtrOr(body.MaxDurationS, 3600),
		CleanupWorktree:  intPtrOr(body.CleanupWorktree, 1),
		Enabled:          intPtrOr(body.Enabled, 1),
	}

	created, err := h.ws.CreateWorkflow(r.Context(), wf)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			errConflict(w, "workflow name already exists")
			return
		}
		errInternalServer(w, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// UpdateWorkflow updates an existing workflow.
// PUT /api/workflows/{workflowID}
func (h *WorkflowHandler) UpdateWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID, err := strconv.ParseInt(chi.URLParam(r, "workflowID"), 10, 64)
	if err != nil {
		errBadRequest(w, "invalid workflow ID")
		return
	}

	existing, err := h.ws.GetWorkflow(r.Context(), workflowID)
	if err != nil {
		errInternalServer(w, err.Error())
		return
	}
	if existing == nil {
		errNotFound(w, "workflow not found")
		return
	}

	// Decode as raw map for dynamic update
	var raw map[string]json.RawMessage
	if err := decodeJSON(r, &raw); err != nil {
		errBadRequest(w, "invalid JSON")
		return
	}

	fields := make(map[string]interface{})

	// Handle name update with validation
	if nameRaw, ok := raw["name"]; ok {
		var name string
		json.Unmarshal(nameRaw, &name)
		if !workflowNameRe.MatchString(name) {
			errBadRequest(w, "name contains invalid characters")
			return
		}
		fields["name"] = name
	}

	// Handle simple string fields
	for _, key := range []string{"description", "repo_path", "base_branch"} {
		if v, ok := raw[key]; ok {
			var s string
			json.Unmarshal(v, &s)
			fields[key] = s
		}
	}

	// Handle repo_path validation and canonicalization
	if rp, ok := fields["repo_path"]; ok {
		if rpStr, ok := rp.(string); ok && rpStr != "" {
			cleaned := filepath.Clean(rpStr)
			resolved, err := filepath.EvalSymlinks(cleaned)
			if err != nil {
				errBadRequest(w, "repo_path does not exist")
				return
			}
			info, err := os.Stat(resolved)
			if err != nil || !info.IsDir() {
				errBadRequest(w, "repo_path does not exist")
				return
			}
			fields["repo_path"] = resolved
		}
	}

	// Handle int fields
	for _, key := range []string{"max_duration_s", "cleanup_worktree", "enabled"} {
		if v, ok := raw[key]; ok {
			var n int
			json.Unmarshal(v, &n)
			fields[key] = n
		}
	}

	// Validate max_duration_s
	if v, ok := fields["max_duration_s"]; ok {
		if n, ok := v.(int); ok && (n <= 0 || n > 86400) {
			errBadRequest(w, "invalid max_duration")
			return
		}
	}

	// Handle steps
	if stepsRaw, ok := raw["steps"]; ok {
		var steps []json.RawMessage
		if err := json.Unmarshal(stepsRaw, &steps); err != nil {
			errBadRequest(w, "invalid steps")
			return
		}
		if len(steps) == 0 {
			errBadRequest(w, "at least one step is required")
			return
		}
		if len(steps) > 20 {
			errBadRequest(w, "maximum 20 steps allowed")
			return
		}

		// Resolve default_agent for validation
		defaultAgent := json.RawMessage(existing.DefaultAgentJSON)
		if daRaw, ok := raw["default_agent"]; ok {
			defaultAgent = daRaw
		}
		if errMsg := validateSteps(steps, defaultAgent); errMsg != "" {
			errBadRequest(w, errMsg)
			return
		}

		stepsJSON, err := store.StepsJSONFromRaw(steps)
		if err != nil {
			errBadRequest(w, "invalid steps")
			return
		}
		fields["steps_json"] = stepsJSON
	}

	// Handle default_agent
	if daRaw, ok := raw["default_agent"]; ok {
		fields["default_agent_json"] = string(daRaw)
	}

	updated, err := h.ws.UpdateWorkflow(r.Context(), workflowID, fields)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			errConflict(w, "workflow name already exists")
			return
		}
		errInternalServer(w, err.Error())
		return
	}
	if updated == nil {
		errNotFound(w, "workflow not found")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteWorkflow deletes a workflow and its run history.
// DELETE /api/workflows/{workflowID}
func (h *WorkflowHandler) DeleteWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID, _ := strconv.ParseInt(chi.URLParam(r, "workflowID"), 10, 64)
	if err := h.ws.DeleteWorkflow(r.Context(), workflowID); err != nil {
		errInternalServer(w, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ── Trigger & Runs ───────────────────────────────────────────────────

// TriggerWorkflow triggers a workflow run by ID.
// POST /api/workflows/{workflowID}/trigger
func (h *WorkflowHandler) TriggerWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID, err := strconv.ParseInt(chi.URLParam(r, "workflowID"), 10, 64)
	if err != nil {
		errBadRequest(w, "invalid workflow ID")
		return
	}
	h.triggerByID(w, r, workflowID)
}

// TriggerWorkflowByName triggers a workflow run by name.
// POST /api/workflows/by-name/{name}/trigger
func (h *WorkflowHandler) TriggerWorkflowByName(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	wf, err := h.ws.GetWorkflowByName(r.Context(), name)
	if err != nil {
		errInternalServer(w, err.Error())
		return
	}
	if wf == nil {
		errNotFound(w, "workflow not found")
		return
	}
	h.triggerByID(w, r, wf.ID)
}

func (h *WorkflowHandler) triggerByID(w http.ResponseWriter, r *http.Request, workflowID int64) {
	wf, err := h.ws.GetWorkflow(r.Context(), workflowID)
	if err != nil {
		errInternalServer(w, err.Error())
		return
	}
	if wf == nil {
		errNotFound(w, "workflow not found")
		return
	}
	if wf.Enabled == 0 {
		errConflict(w, "workflow is disabled")
		return
	}

	var body struct {
		TriggerType string          `json:"trigger_type"`
		Context     json.RawMessage `json:"context"`
	}
	// Body is optional for trigger
	decodeJSON(r, &body)

	if body.TriggerType == "" {
		body.TriggerType = "api"
	}

	var triggerCtx *string
	if body.Context != nil {
		s := string(body.Context)
		triggerCtx = &s
	}

	// Use the runner if available (it creates the run and starts execution)
	if h.runner != nil {
		run, err := h.runner.TriggerWorkflow(r.Context(), wf, body.TriggerType, triggerCtx)
		if err != nil {
			errInternalServer(w, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"run_id":        run.ID,
			"workflow_id":   wf.ID,
			"workflow_name": wf.Name,
			"status":        run.Status,
			"trigger_type":  run.TriggerType,
			"created_at":    run.CreatedAt,
		})
		return
	}

	// Fallback: create run record only (no execution)
	run, err := h.ws.CreateWorkflowRun(r.Context(), workflowID, body.TriggerType, triggerCtx)
	if err != nil {
		errInternalServer(w, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"run_id":        run.ID,
		"workflow_id":   wf.ID,
		"workflow_name": wf.Name,
		"status":        run.Status,
		"trigger_type":  run.TriggerType,
		"created_at":    run.CreatedAt,
	})
}

// ListWorkflowRuns returns runs for a specific workflow.
// GET /api/workflows/{workflowID}/runs
func (h *WorkflowHandler) ListWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	workflowID, _ := strconv.ParseInt(chi.URLParam(r, "workflowID"), 10, 64)
	limit := queryInt(r, "limit", 20)
	offset := queryInt(r, "offset", 0)
	status := strPtr(r.URL.Query().Get("status"))

	runs, err := h.ws.ListRunsForWorkflow(r.Context(), workflowID, limit, offset, status)
	if err != nil {
		errInternalServer(w, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": emptyIfNil(runs)})
}

// ListRecentRuns returns recent runs across all workflows.
// GET /api/workflows/runs/recent
func (h *WorkflowHandler) ListRecentRuns(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 20)
	offset := queryInt(r, "offset", 0)
	status := strPtr(r.URL.Query().Get("status"))

	runs, err := h.ws.ListRecentRuns(r.Context(), limit, offset, status)
	if err != nil {
		errInternalServer(w, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": emptyIfNil(runs)})
}

// GetWorkflowRun returns a single run with step results.
// GET /api/workflows/runs/{runID}
func (h *WorkflowHandler) GetWorkflowRun(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.ParseInt(chi.URLParam(r, "runID"), 10, 64)
	if err != nil {
		errBadRequest(w, "invalid run ID")
		return
	}
	run, err := h.ws.GetWorkflowRun(r.Context(), runID)
	if err != nil {
		errInternalServer(w, err.Error())
		return
	}
	if run == nil {
		errNotFound(w, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// KillWorkflowRun kills a running workflow.
// POST /api/workflows/runs/{runID}/kill
func (h *WorkflowHandler) KillWorkflowRun(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.ParseInt(chi.URLParam(r, "runID"), 10, 64)
	if err != nil {
		errBadRequest(w, "invalid run ID")
		return
	}
	run, err := h.ws.GetWorkflowRunDirect(r.Context(), runID)
	if err != nil {
		errInternalServer(w, err.Error())
		return
	}
	if run == nil {
		errNotFound(w, "run not found")
		return
	}
	if run.Status != "pending" && run.Status != "running" {
		errConflict(w, "run is not active")
		return
	}

	// Use the runner for proper process/session cleanup if available
	if h.runner != nil {
		if h.runner.KillRun(r.Context(), runID) {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "run_id": runID, "status": "killed"})
			return
		}
	}

	// Fallback: mark killed in DB only
	errMsg := "killed via API"
	if err := h.ws.SetRunStatus(r.Context(), runID, "killed", &errMsg); err != nil {
		errInternalServer(w, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "run_id": runID, "status": "killed"})
}

// ── Step Validation ──────────────────────────────────────────────────

// validateSteps validates step definitions per the spec rules.
func validateSteps(steps []json.RawMessage, defaultAgent json.RawMessage) string {
	stepNames := make(map[string]bool)

	for i, stepRaw := range steps {
		var step struct {
			Name              string          `json:"name"`
			Type              string          `json:"type"`
			Command           string          `json:"command"`
			Prompt            string          `json:"prompt"`
			TimeoutS          *int            `json:"timeout_s"`
			ContinueOnFailure bool            `json:"continue_on_failure"`
			Agent             json.RawMessage `json:"agent"`
		}
		if err := json.Unmarshal(stepRaw, &step); err != nil {
			return fmt.Sprintf("step %d: invalid JSON", i)
		}

		// Validate name
		if step.Name == "" {
			return fmt.Sprintf("step %d: name is required", i)
		}
		if stepNames[step.Name] {
			return "duplicate step name"
		}
		stepNames[step.Name] = true

		// Validate type
		switch step.Type {
		case "shell":
			if step.Command == "" {
				return "shell step missing command"
			}
		case "agent":
			if step.Prompt == "" {
				return "agent step missing prompt"
			}
			// Check agent_type is resolvable
			hasAgentType := false
			if step.Agent != nil {
				var agentCfg struct {
					AgentType string `json:"agent_type"`
				}
				json.Unmarshal(step.Agent, &agentCfg)
				hasAgentType = agentCfg.AgentType != ""
			}
			if !hasAgentType && defaultAgent != nil {
				var daCfg struct {
					AgentType string `json:"agent_type"`
				}
				json.Unmarshal(defaultAgent, &daCfg)
				hasAgentType = daCfg.AgentType != ""
			}
			if !hasAgentType {
				return "agent step missing agent_type"
			}
		default:
			return fmt.Sprintf("step %d: type must be 'shell' or 'agent'", i)
		}

		// Validate timeout_s
		if step.TimeoutS != nil && (*step.TimeoutS <= 0 || *step.TimeoutS > 86400) {
			return "invalid timeout"
		}

		// Validate template references
		text := step.Command + step.Prompt
		if errMsg := validateTemplateRefs(text, i, len(steps)); errMsg != "" {
			return errMsg
		}
	}
	return ""
}

// validateTemplateRefs checks template variable references in step text.
func validateTemplateRefs(text string, stepIndex, totalSteps int) string {
	// Check {{prev_*}} references on step 0
	if stepIndex == 0 {
		for _, tmpl := range []string{"{{prev_dir}}", "{{prev_stdout}}", "{{prev_stderr}}"} {
			if strings.Contains(text, tmpl) {
				return "step 0 cannot reference previous step"
			}
		}
	}

	// Check {{step_N_*}} references
	// Match patterns like {{step_0_dir}}, {{step_2_stdout}}
	for i := 0; i < totalSteps; i++ {
		prefix := fmt.Sprintf("{{step_%d_", i)
		if strings.Contains(text, prefix) {
			if i >= stepIndex {
				return "template references invalid step index"
			}
		}
	}

	return ""
}
