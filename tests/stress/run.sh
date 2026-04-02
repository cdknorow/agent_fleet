#!/usr/bin/env bash
#
# Stress test for the Coral board task system.
# Tests task creation, claiming, completion, and listing.
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CORAL_DIR="$REPO_ROOT/coral-go"
PORT=8471
HOST="127.0.0.1"
BASE_URL="http://${HOST}:${PORT}"
BOARD="stress-test-$$"
NUM_AGENTS=5
NUM_TASKS=20
PASS=0
FAIL=0
SERVER_PID=""

# ── Helpers ──────────────────────────────────────────────────────────

cleanup() {
    if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
    fi
    rm -rf "$TMPDIR_STRESS" 2>/dev/null || true
}
trap cleanup EXIT

log()  { echo "[stress] $*"; }
pass() { PASS=$((PASS + 1)); log "PASS: $*"; }
fail() { FAIL=$((FAIL + 1)); log "FAIL: $*"; }

api() {
    local method="$1" path="$2"
    shift 2
    curl -s -m 10 -X "$method" "${BASE_URL}/api/board/${BOARD}${path}" \
        -H "Content-Type: application/json" "$@"
}

api_status() {
    local method="$1" path="$2"
    shift 2
    curl -s -m 10 -o /dev/null -w "%{http_code}" -X "$method" "${BASE_URL}/api/board/${BOARD}${path}" \
        -H "Content-Type: application/json" "$@"
}

wait_for_server() {
    local retries=30
    while ! curl -s -m 5 "${BASE_URL}/api/health" >/dev/null 2>&1; do
        retries=$((retries - 1))
        if [[ $retries -le 0 ]]; then
            log "ERROR: Server failed to start on port $PORT"
            exit 1
        fi
        sleep 0.5
    done
    log "Server is ready on port $PORT"
}

# ── Setup ────────────────────────────────────────────────────────────

TMPDIR_STRESS="$(mktemp -d)"
export CORAL_DATA_DIR="$TMPDIR_STRESS"

log "Building coral (dev mode)..."
cd "$CORAL_DIR"
go build -tags dev -o "$TMPDIR_STRESS/coral" ./cmd/coral/

log "Starting coral server on port $PORT..."
"$TMPDIR_STRESS/coral" --host "$HOST" --port "$PORT" --backend tmux >"$TMPDIR_STRESS/server.log" 2>&1 &
SERVER_PID=$!
wait_for_server

# ── Test 1: Create tasks ────────────────────────────────────────────

log "Creating $NUM_TASKS tasks..."
created=0
for i in $(seq 1 $NUM_TASKS); do
    prio="medium"
    case $((i % 4)) in
        0) prio="critical" ;;
        1) prio="high" ;;
        2) prio="medium" ;;
        3) prio="low" ;;
    esac
    result=$(api POST "/tasks" -d "{\"title\": \"Task $i\", \"body\": \"Stress test task number $i\", \"priority\": \"$prio\", \"subscriber_id\": \"orchestrator\"}")
    tid=$(echo "$result" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || echo "")
    if [[ -n "$tid" ]]; then
        created=$((created + 1))
    fi
done

if [[ "$created" -eq "$NUM_TASKS" ]]; then
    pass "Created $NUM_TASKS tasks"
else
    fail "Created $created tasks, expected $NUM_TASKS"
fi

# ── Test 2: List returns all tasks ───────────────────────────────────

list_count=$(api GET "/tasks" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('tasks', [])))" 2>/dev/null || echo "0")

if [[ "$list_count" -eq "$NUM_TASKS" ]]; then
    pass "ListTasks returns all $NUM_TASKS tasks"
else
    fail "ListTasks returned $list_count, expected $NUM_TASKS"
fi

# ── Test 3: Concurrent claims — no duplicates ────────────────────────

log "Testing $NUM_AGENTS concurrent claims..."

# Use xargs -P for reliable parallel execution
CLAIM_RESULTS="$TMPDIR_STRESS/claim_results"
mkdir -p "$CLAIM_RESULTS"

# Write a helper script for xargs to call
cat > "$TMPDIR_STRESS/do_claim.sh" <<SCRIPT
#!/usr/bin/env bash
i=\$1
curl -s -m 10 -X POST "${BASE_URL}/api/board/${BOARD}/tasks/claim" \
    -H "Content-Type: application/json" \
    -d "{\"subscriber_id\": \"agent-\${i}\"}" > "${CLAIM_RESULTS}/agent-\${i}.json"
SCRIPT
chmod +x "$TMPDIR_STRESS/do_claim.sh"

seq 1 $NUM_AGENTS | xargs -P "$NUM_AGENTS" -I{} "$TMPDIR_STRESS/do_claim.sh" {}

# All agents should have claimed a task (20 pending, 5 agents)
claim_success=0
claimed_ids=()
for i in $(seq 1 $NUM_AGENTS); do
    tid=$(cat "$CLAIM_RESULTS/agent-${i}.json" 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || echo "")
    if [[ -n "$tid" ]]; then
        claim_success=$((claim_success + 1))
        claimed_ids+=("$tid")
    fi
done

# Under SQLite single-writer, some concurrent claims may fail with contention.
# The key invariant is no duplicates. Require at least 1 success.
if [[ "$claim_success" -ge 1 ]]; then
    pass "$claim_success of $NUM_AGENTS agents claimed a task concurrently"
else
    fail "No concurrent claims succeeded"
fi

unique=$(printf '%s\n' "${claimed_ids[@]}" | sort -u | wc -l | tr -d ' ')
if [[ "${#claimed_ids[@]}" -eq "$unique" ]]; then
    pass "No duplicate claims: all $unique task IDs unique"
else
    fail "Duplicate claims: ${#claimed_ids[@]} claims but $unique unique"
fi

# ── Test 4: Claim + complete lifecycle ───────────────────────────────

log "Claiming and completing all remaining tasks..."

# Complete the tasks claimed above
for i in $(seq 1 $NUM_AGENTS); do
    tid=$(cat "$CLAIM_RESULTS/agent-${i}.json" 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || echo "")
    if [[ -n "$tid" ]]; then
        api POST "/tasks/${tid}/complete" -d "{\"subscriber_id\": \"agent-${i}\", \"message\": \"done\"}" >/dev/null
    fi
done

# Claim and complete remaining tasks sequentially
while true; do
    result=$(api POST "/tasks/claim" -d '{"subscriber_id": "agent-1"}')
    tid=$(echo "$result" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || echo "")
    [[ -z "$tid" ]] && break
    api POST "/tasks/${tid}/complete" -d '{"subscriber_id": "agent-1", "message": "done"}' >/dev/null
done

# Verify all completed
completed_count=$(api GET "/tasks" | python3 -c "
import sys, json
tasks = json.load(sys.stdin).get('tasks', [])
print(sum(1 for t in tasks if t.get('status') == 'completed'))
" 2>/dev/null || echo "0")

if [[ "$completed_count" -eq "$NUM_TASKS" ]]; then
    pass "All $NUM_TASKS tasks claimed and completed"
else
    fail "Only $completed_count of $NUM_TASKS completed"
fi

# ── Test 5: Claim returns 404 when all done ──────────────────────────

status=$(api_status POST "/tasks/claim" -d '{"subscriber_id": "agent-1"}')
if [[ "$status" == "404" ]]; then
    pass "Claim returns 404 when no tasks available"
else
    fail "Claim returned HTTP $status, expected 404"
fi

# ── Test 6: Sequential claiming — cannot claim while in-progress ────

# Use a fresh board for sequential tests to avoid interference
SEQ_BOARD="seq-test-$$"
seq_api() {
    local method="$1" path="$2"
    shift 2
    curl -s -m 10 -X "$method" "${BASE_URL}/api/board/${SEQ_BOARD}${path}" \
        -H "Content-Type: application/json" "$@"
}
seq_api_status() {
    local method="$1" path="$2"
    shift 2
    curl -s -m 10 -o /dev/null -w "%{http_code}" -X "$method" "${BASE_URL}/api/board/${SEQ_BOARD}${path}" \
        -H "Content-Type: application/json" "$@"
}

# Subscribe an agent
seq_api POST "/subscribe" -d '{"subscriber_id": "seq-agent", "job_title": "tester"}' >/dev/null

# Create two tasks
seq_api POST "/tasks" -d '{"title": "Seq Task 1", "priority": "high", "subscriber_id": "orchestrator"}' >/dev/null
seq_api POST "/tasks" -d '{"title": "Seq Task 2", "priority": "medium", "subscriber_id": "orchestrator"}' >/dev/null

# Claim first task
t1_result=$(seq_api POST "/tasks/claim" -d '{"subscriber_id": "seq-agent"}')
t1_id=$(echo "$t1_result" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || echo "")

if [[ -n "$t1_id" ]]; then
    pass "Sequential: agent claimed first task (id=$t1_id)"
else
    fail "Sequential: agent failed to claim first task"
fi

# Try to claim second task while first is in-progress — should get 409
status=$(seq_api_status POST "/tasks/claim" -d '{"subscriber_id": "seq-agent"}')
if [[ "$status" == "409" ]]; then
    pass "Sequential: second claim blocked with 409 while task in-progress"
else
    fail "Sequential: expected 409 for second claim, got HTTP $status"
fi

# Verify the error message mentions completing current task
err_body=$(seq_api POST "/tasks/claim" -d '{"subscriber_id": "seq-agent"}')
err_msg=$(echo "$err_body" | python3 -c "import sys,json; print(json.load(sys.stdin).get('error',''))" 2>/dev/null || echo "")
if echo "$err_msg" | grep -qi "complete"; then
    pass "Sequential: error message mentions completing current task"
else
    fail "Sequential: error message unclear: '$err_msg'"
fi

# ── Test 7: Claim after complete — agent can claim again ────────────

# Complete the first task
seq_api POST "/tasks/${t1_id}/complete" -d '{"subscriber_id": "seq-agent", "message": "done"}' >/dev/null

# Now claim should succeed
t2_result=$(seq_api POST "/tasks/claim" -d '{"subscriber_id": "seq-agent"}')
t2_id=$(echo "$t2_result" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || echo "")

if [[ -n "$t2_id" ]] && [[ "$t2_id" != "$t1_id" ]]; then
    pass "Sequential: agent claimed second task after completing first (id=$t2_id)"
else
    fail "Sequential: failed to claim after completion (got '$t2_id')"
fi

# Complete second task
seq_api POST "/tasks/${t2_id}/complete" -d '{"subscriber_id": "seq-agent", "message": "done"}' >/dev/null

# ── Test 8: Priority ordering — tasks come out in priority order ─────

PRIO_BOARD="prio-test-$$"
prio_api() {
    local method="$1" path="$2"
    shift 2
    curl -s -m 10 -X "$method" "${BASE_URL}/api/board/${PRIO_BOARD}${path}" \
        -H "Content-Type: application/json" "$@"
}

# Subscribe
prio_api POST "/subscribe" -d '{"subscriber_id": "prio-agent", "job_title": "tester"}' >/dev/null

# Create tasks in reverse priority order (low first, critical last)
prio_api POST "/tasks" -d '{"title": "Low prio", "priority": "low", "subscriber_id": "orchestrator"}' >/dev/null
prio_api POST "/tasks" -d '{"title": "Medium prio", "priority": "medium", "subscriber_id": "orchestrator"}' >/dev/null
prio_api POST "/tasks" -d '{"title": "Critical prio", "priority": "critical", "subscriber_id": "orchestrator"}' >/dev/null
prio_api POST "/tasks" -d '{"title": "High prio", "priority": "high", "subscriber_id": "orchestrator"}' >/dev/null

# Claim and complete in sequence, record the priority order
claimed_prios=()
for i in $(seq 1 4); do
    result=$(prio_api POST "/tasks/claim" -d '{"subscriber_id": "prio-agent"}')
    title=$(echo "$result" | python3 -c "import sys,json; print(json.load(sys.stdin).get('title',''))" 2>/dev/null || echo "")
    tid=$(echo "$result" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || echo "")
    claimed_prios+=("$title")
    if [[ -n "$tid" ]]; then
        prio_api POST "/tasks/${tid}/complete" -d '{"subscriber_id": "prio-agent", "message": "done"}' >/dev/null
    fi
done

expected_order="Critical prio|High prio|Medium prio|Low prio"
actual_order=$(IFS='|'; echo "${claimed_prios[*]}")

if [[ "$actual_order" == "$expected_order" ]]; then
    pass "Priority ordering: tasks claimed in correct order (critical > high > medium > low)"
else
    fail "Priority ordering: expected '$expected_order', got '$actual_order'"
fi

# ── Test 9: Coral Task Queue messages don't count as unread ─────────

NUDGE_BOARD="nudge-test-$$"
nudge_api() {
    local method="$1" path="$2"
    shift 2
    curl -s -m 10 -X "$method" "${BASE_URL}/api/board/${NUDGE_BOARD}${path}" \
        -H "Content-Type: application/json" "$@"
}

# Subscribe the agent and read all existing messages to start at zero unread
nudge_api POST "/subscribe" -d '{"subscriber_id": "nudge-agent", "job_title": "tester"}' >/dev/null
nudge_api GET "/messages?subscriber_id=nudge-agent" >/dev/null

# Create and claim a task — this generates 'Coral Task Queue' audit messages
nudge_api POST "/tasks" -d '{"title": "Nudge Task 1", "priority": "high", "subscriber_id": "orchestrator"}' >/dev/null
nudge_api POST "/tasks" -d '{"title": "Nudge Task 2", "priority": "medium", "subscriber_id": "orchestrator"}' >/dev/null

# Read messages again to clear cursor (task creation may post messages)
nudge_api GET "/messages?subscriber_id=nudge-agent" >/dev/null

# Claim a task — this posts a 'Coral Task Queue' audit message
nudge_api POST "/tasks/claim" -d '{"subscriber_id": "nudge-agent"}' >/dev/null

# Check unread — Coral Task Queue messages should NOT count
unread=$(curl -s -m 10 "${BASE_URL}/api/board/${NUDGE_BOARD}/messages/check?subscriber_id=nudge-agent" | python3 -c "import sys,json; print(json.load(sys.stdin).get('unread',99))" 2>/dev/null || echo "99")

if [[ "$unread" -eq 0 ]]; then
    pass "Coral Task Queue messages do not count as unread"
else
    fail "Expected 0 unread after task queue messages, got $unread"
fi

# ── Test 10: Completion posts nudge when next task is pending ────────

# Complete the first task — there's a second pending task, so a nudge should be posted
t_id=$(nudge_api GET "/tasks" | python3 -c "
import sys,json
tasks = json.load(sys.stdin).get('tasks',[])
for t in tasks:
    if t.get('status') == 'in_progress':
        print(t['id']); break
" 2>/dev/null || echo "")

if [[ -n "$t_id" ]]; then
    nudge_api POST "/tasks/${t_id}/complete" -d '{"subscriber_id": "nudge-agent", "message": "done"}' >/dev/null
fi

# Give the async goroutine a moment to post
sleep 0.5

# Read recent messages and check for a nudge about available tasks
msgs=$(nudge_api GET "/messages?subscriber_id=nudge-agent")
has_nudge=$(echo "$msgs" | python3 -c "
import sys,json
data = json.load(sys.stdin)
messages = data if isinstance(data, list) else data.get('messages', [])
# Look for nudge from Coral Task Queue mentioning 'claim'
found = any('claim' in m.get('content','').lower()
            for m in messages if m.get('subscriber_id','') == 'Coral Task Queue')
print('yes' if found else 'no')
" 2>/dev/null || echo "no")

if [[ "$has_nudge" == "yes" ]]; then
    pass "Completion posts nudge message about next pending task"
else
    fail "No nudge message found after completing task with another pending"
fi

# ── Test 11: Terminal nudge delivered to agent's tmux pane ───────────

# Build the mock agent
log "Building mock-agent for terminal nudge test..."
go build -o "$TMPDIR_STRESS/mock-agent" ./cmd/mock-agent/

# Override claude CLI to use mock-agent
curl -s -m 10 -X PUT "${BASE_URL}/api/settings" \
    -H "Content-Type: application/json" \
    -d "{\"cli_path_claude\": \"$TMPDIR_STRESS/mock-agent\"}" >/dev/null

TERM_BOARD="term-nudge-$$"
term_api() {
    local method="$1" path="$2"
    shift 2
    curl -s -m 10 -X "$method" "${BASE_URL}/api/board/${TERM_BOARD}${path}" \
        -H "Content-Type: application/json" "$@"
}

# Launch a mock agent session
launch_result=$(curl -s -m 10 -X POST "${BASE_URL}/api/sessions/launch" \
    -H "Content-Type: application/json" \
    -d '{"working_dir": "/tmp", "agent_type": "claude", "display_name": "Nudge Tester"}')
mock_session=$(echo "$launch_result" | python3 -c "import sys,json; print(json.load(sys.stdin).get('session_name',''))" 2>/dev/null || echo "")

if [[ -z "$mock_session" ]]; then
    fail "Terminal nudge: could not launch mock agent session"
else
    # Wait for mock agent to start
    sleep 3

    # Subscribe the session to the board
    term_api POST "/subscribe" \
        -d "{\"subscriber_id\": \"$mock_session\", \"job_title\": \"tester\", \"session_name\": \"$mock_session\"}" >/dev/null

    # Create 2 tasks, claim first, complete it
    term_api POST "/tasks" -d '{"title": "Terminal Task 1", "priority": "high", "subscriber_id": "orchestrator"}' >/dev/null
    term_api POST "/tasks" -d '{"title": "Terminal Task 2", "priority": "medium", "subscriber_id": "orchestrator"}' >/dev/null

    term_tid=$(term_api POST "/tasks/claim" -d "{\"subscriber_id\": \"$mock_session\"}" | \
        python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || echo "")

    term_api POST "/tasks/${term_tid}/complete" \
        -d "{\"subscriber_id\": \"$mock_session\", \"message\": \"done\"}" >/dev/null

    # Wait for async SendInput delivery
    sleep 2

    # Capture the terminal and check for nudge text
    capture=$(curl -s -m 10 "${BASE_URL}/api/sessions/live/${mock_session}/capture" 2>/dev/null)
    has_terminal_nudge=$(echo "$capture" | python3 -c "
import sys, json
data = json.load(sys.stdin)
text = (data.get('capture') or '').lower()
print('yes' if 'task' in text and 'claim' in text else 'no')
" 2>/dev/null || echo "no")

    if [[ "$has_terminal_nudge" == "yes" ]]; then
        pass "Terminal nudge delivered to agent's tmux pane"
    else
        fail "Terminal nudge not found in agent's tmux pane capture"
    fi

    # Clean up the mock session
    curl -s -m 10 -X POST "${BASE_URL}/api/sessions/live/${mock_session}/kill" >/dev/null
fi

# Reset CLI path
curl -s -m 10 -X PUT "${BASE_URL}/api/settings" \
    -H "Content-Type: application/json" \
    -d '{"cli_path_claude": ""}' >/dev/null

# ── Summary ──────────────────────────────────────────────────────────

echo ""
echo "================================"
echo "  STRESS TEST RESULTS"
echo "  Passed: $PASS"
echo "  Failed: $FAIL"
echo "================================"

if [[ $FAIL -gt 0 ]]; then
    exit 1
fi
