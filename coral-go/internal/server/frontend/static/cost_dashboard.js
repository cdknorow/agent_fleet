/* Token Usage Dashboard — proxy token tracking and cost UI */

import { showView, escapeHtml, escapeAttr } from './utils.js';
import { state } from './state.js';

function _formatTokens(n) {
    if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M';
    if (n >= 1000) return (n / 1000).toFixed(1) + 'K';
    return String(n);
}

function _formatCost(c) {
    if (c === 0) return '$0.00';
    if (c < 0.01) return '$' + c.toFixed(4);
    return '$' + c.toFixed(2);
}

function _formatLatency(ms) {
    if (ms == null) return '\u2014';
    if (ms >= 1000) return (ms / 1000).toFixed(1) + 's';
    return ms + 'ms';
}

function _formatDate(isoStr) {
    if (!isoStr) return '\u2014';
    try {
        const d = new Date(isoStr);
        return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
            + ' ' + d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
    } catch { return isoStr; }
}

function _findSession(sessionId) {
    if (!sessionId || !state.liveSessions) return null;
    return state.liveSessions.find(s => s.session_id === sessionId) || null;
}

let _costRefreshTimer = null;

export async function showCostDashboard() {
    showView('cost-dashboard-view');
    await _refreshCostDashboard();
    stopCostDashboard(); // clear any existing timer
    _costRefreshTimer = setInterval(_refreshCostDashboard, 15000);
}

export function stopCostDashboard() {
    if (_costRefreshTimer) {
        clearInterval(_costRefreshTimer);
        _costRefreshTimer = null;
    }
}

export async function _refreshCostDashboard() {
    const period = document.getElementById('cost-time-range')?.value || 'day';

    // Map period to a 'since' timestamp for the unified token-usage API
    const sinceMap = { 'hour': 1, 'day': 24, 'week': 168, 'month': 720, 'all': 0 };
    const hoursAgo = sinceMap[period] || 24;
    const since = hoursAgo > 0 ? new Date(Date.now() - hoursAgo * 3600000).toISOString() : '';
    const sinceParam = since ? `?since=${encodeURIComponent(since)}` : '';

    try {
        const [summaryResp, reqResp, taskResp] = await Promise.all([
            fetch(`/api/token-usage/summary${sinceParam}`).catch(() => null),
            fetch('/api/proxy/requests?limit=100').catch(() => null),
            fetch('/api/board/tasks').catch(() => null),
        ]);

        if (summaryResp && summaryResp.ok) {
            const data = await summaryResp.json();
            const t = data.totals || {};
            _setText('cost-input-tokens', _formatTokens(t.input_tokens || 0));
            _setText('cost-output-tokens', _formatTokens(t.output_tokens || 0));
            _setText('cost-cache-read', _formatTokens(t.cache_read_tokens || 0));
            _setText('cost-cache-write', _formatTokens(t.cache_write_tokens || 0));
            _setText('cost-total-requests', String(t.num_sessions || 0));
            _setText('cost-total-cost', _formatCost(t.cost_usd || 0));

            // Render per-agent-type breakdown as model table
            _renderModelTable((data.by_agent_type || []).map(a => ({
                model: a.agent_type || 'unknown',
                requests: a.num_sessions || 0,
                input_tokens: a.input_tokens || 0,
                output_tokens: a.output_tokens || 0,
                cache_read_tokens: a.cache_read_tokens || 0,
                cache_write_tokens: a.cache_write_tokens || 0,
                cost_usd: a.cost_usd || 0,
            })));

            // Render per-agent (session) breakdown
            _renderAgentTable((data.by_agent || []).map(a => ({
                session_id: a.session_id,
                agent_name: a.agent_name,
                display_name: a.agent_name || 'unknown',
                board_name: a.board_name || '',
                requests: a.requests || 0,
                input_tokens: a.input_tokens || 0,
                output_tokens: a.output_tokens || 0,
                cache_read_tokens: a.cache_read_tokens || 0,
                cache_write_tokens: a.cache_write_tokens || 0,
                cost_usd: a.cost_usd || 0,
            })));
        }

        if (reqResp && reqResp.ok) {
            const data = await reqResp.json();
            _renderRequestLog(data.requests || []);
        }

        if (taskResp && taskResp.ok) {
            const data = await taskResp.json();
            _renderTaskTable(data.tasks || []);
        }
    } catch (e) {
        console.error('[cost-dashboard] refresh error:', e);
    }
}

export function _costTimeRangeChanged() {
    _refreshCostDashboard();
}

function _setText(id, text) {
    const el = document.getElementById(id);
    if (el) el.textContent = text;
}

function _renderModelTable(rows) {
    const container = document.getElementById('cost-by-model');
    if (!container) return;

    if (rows.length === 0) {
        container.innerHTML = '<div class="cost-empty">No usage data yet</div>';
        return;
    }

    let html = `<table class="cost-table">
        <thead><tr>
            <th>Model</th>
            <th class="cost-col-right">Requests</th>
            <th class="cost-col-right">Input</th>
            <th class="cost-col-right">Output</th>
            <th class="cost-col-right">Cache Read</th>
            <th class="cost-col-right">Cache Write</th>
            <th class="cost-col-right">Cost</th>
        </tr></thead><tbody>`;

    for (const r of rows) {
        html += `<tr>
            <td class="cost-model-name">${escapeHtml(r.model)}</td>
            <td class="cost-col-right">${r.requests}</td>
            <td class="cost-col-right">${_formatTokens(r.input_tokens || 0)}</td>
            <td class="cost-col-right">${_formatTokens(r.output_tokens || 0)}</td>
            <td class="cost-col-right">${_formatTokens(r.cache_read_tokens || 0)}</td>
            <td class="cost-col-right">${_formatTokens(r.cache_write_tokens || 0)}</td>
            <td class="cost-col-right cost-cost-cell">${_formatCost(r.cost_usd)}</td>
        </tr>`;
    }

    html += '</tbody></table>';
    container.innerHTML = html;
}

function _renderAgentTable(rows) {
    const container = document.getElementById('cost-by-agent');
    if (!container) return;

    if (rows.length === 0) {
        container.innerHTML = '<div class="cost-empty">No usage data yet</div>';
        return;
    }

    let html = `<table class="cost-table">
        <thead><tr>
            <th>Agent</th>
            <th>Team</th>
            <th class="cost-col-right">Requests</th>
            <th class="cost-col-right">Input</th>
            <th class="cost-col-right">Output</th>
            <th class="cost-col-right">Cache Read</th>
            <th class="cost-col-right">Cache Write</th>
            <th class="cost-col-right">Cost</th>
        </tr></thead><tbody>`;

    for (const r of rows) {
        const session = _findSession(r.session_id);
        const displayName = r.display_name || (session && session.display_name) || r.agent_name || r.session_id || '\u2014';
        let nameHtml;
        if (session) {
            nameHtml = `<a href="#" class="cost-agent-link" onclick="event.preventDefault(); switchNavTab('agents'); selectLiveSession('${escapeAttr(session.name)}', '${escapeAttr(session.agent_type)}', '${escapeAttr(session.session_id)}')">${escapeHtml(displayName)}</a>`;
        } else if (r.session_id) {
            const endedSuffix = r.is_live === false ? ' <span class="cost-agent-ended">(ended)</span>' : '';
            nameHtml = `<a href="#" class="cost-agent-link cost-agent-terminated" onclick="event.preventDefault(); switchNavTab('history'); selectHistorySession('${escapeAttr(r.session_id)}')">${escapeHtml(displayName)}</a>${endedSuffix}`;
        } else {
            nameHtml = escapeHtml(displayName);
        }
        const teamHtml = r.board_name ? escapeHtml(r.board_name) : '\u2014';
        html += `<tr data-session-id="${escapeAttr(r.session_id || '')}">
            <td class="cost-agent-name">${nameHtml}</td>
            <td class="cost-agent-team">${teamHtml}</td>
            <td class="cost-col-right">${r.requests}</td>
            <td class="cost-col-right">${_formatTokens(r.input_tokens || 0)}</td>
            <td class="cost-col-right">${_formatTokens(r.output_tokens || 0)}</td>
            <td class="cost-col-right">${_formatTokens(r.cache_read_tokens || 0)}</td>
            <td class="cost-col-right">${_formatTokens(r.cache_write_tokens || 0)}</td>
            <td class="cost-col-right cost-cost-cell">${_formatCost(r.cost_usd)}</td>
        </tr>`;
    }

    html += '</tbody></table>';
    container.innerHTML = html;

    // Attach hover chart handlers
    _attachAgentChartHovers(container);
}

// ── Turns-vs-Cost Hover Chart ─────────────────────────────────

const _turnsCache = {};
let _chartTooltip = null;

function _ensureChartTooltip() {
    if (_chartTooltip) return _chartTooltip;
    _chartTooltip = document.createElement('div');
    _chartTooltip.className = 'agent-cost-chart-tooltip';
    document.body.appendChild(_chartTooltip);
    return _chartTooltip;
}

function _attachAgentChartHovers(container) {
    const rows = container.querySelectorAll('tr[data-session-id]');
    for (const row of rows) {
        const sessionId = row.getAttribute('data-session-id');
        if (!sessionId) continue;

        row.addEventListener('mouseenter', async (e) => {
            const tooltip = _ensureChartTooltip();

            // Fetch turns data (cached)
            if (!_turnsCache[sessionId]) {
                try {
                    const resp = await fetch(`/api/token-usage/session/${encodeURIComponent(sessionId)}/turns`);
                    if (resp.ok) {
                        const data = await resp.json();
                        _turnsCache[sessionId] = data.turns || [];
                    } else {
                        _turnsCache[sessionId] = [];
                    }
                } catch { _turnsCache[sessionId] = []; }
            }

            const turns = _turnsCache[sessionId];
            if (turns.length < 2) { tooltip.style.display = 'none'; return; }

            tooltip.innerHTML = `<div class="chart-title">Cumulative Cost over ${turns.length} turns</div>` + _renderCostSVG(turns);

            // Position near cursor
            const x = Math.min(e.clientX + 16, window.innerWidth - 340);
            const y = Math.min(e.clientY - 100, window.innerHeight - 220);
            tooltip.style.left = x + 'px';
            tooltip.style.top = Math.max(8, y) + 'px';
            tooltip.style.display = 'block';
        });

        row.addEventListener('mouseleave', () => {
            const tooltip = _ensureChartTooltip();
            tooltip.style.display = 'none';
        });
    }
}

function _renderCostSVG(turns) {
    const W = 300, H = 160, PAD = 32;
    const plotW = W - PAD * 2, plotH = H - PAD * 2;

    const maxCost = turns[turns.length - 1].cumulative_cost || 1;
    const maxTurn = turns.length;

    // Build polyline points
    const points = turns.map((t, i) => {
        const x = PAD + (i / (maxTurn - 1)) * plotW;
        const y = PAD + plotH - (t.cumulative_cost / maxCost) * plotH;
        return `${x.toFixed(1)},${y.toFixed(1)}`;
    });

    // Area fill points (close path at bottom)
    const areaPoints = [...points, `${(PAD + plotW).toFixed(1)},${(PAD + plotH).toFixed(1)}`, `${PAD.toFixed(1)},${(PAD + plotH).toFixed(1)}`];

    return `<svg width="${W}" height="${H}" viewBox="0 0 ${W} ${H}">
        <line class="chart-grid" x1="${PAD}" y1="${PAD}" x2="${PAD}" y2="${PAD + plotH}" />
        <line class="chart-grid" x1="${PAD}" y1="${PAD + plotH}" x2="${PAD + plotW}" y2="${PAD + plotH}" />
        <polygon class="chart-area" points="${areaPoints.join(' ')}" />
        <polyline class="chart-line" points="${points.join(' ')}" />
        <text class="chart-axis-label" x="${PAD - 4}" y="${PAD + 4}" text-anchor="end">${_formatCost(maxCost)}</text>
        <text class="chart-axis-label" x="${PAD - 4}" y="${PAD + plotH + 4}" text-anchor="end">$0</text>
        <text class="chart-axis-label" x="${PAD}" y="${PAD + plotH + 16}" text-anchor="start">1</text>
        <text class="chart-axis-label" x="${PAD + plotW}" y="${PAD + plotH + 16}" text-anchor="end">${maxTurn}</text>
    </svg>`;
}

// Cached tasks for click-to-detail
let _costTaskCache = [];

function _showCostTaskDetail(taskId) {
    // Merge into state so showTaskDetailModal can find it
    const existing = state.currentBoardTasks || [];
    const task = _costTaskCache.find(t => t.id === taskId);
    if (task && !existing.find(t => t.id === taskId)) {
        state.currentBoardTasks = [...existing, task];
    }
    window.showTaskDetailModal(taskId);
}
window._showCostTaskDetail = _showCostTaskDetail;

function _renderTaskTable(tasks) {
    const container = document.getElementById('cost-by-task');
    if (!container) return;

    // Only show tasks with cost data
    const rows = tasks
        .filter(t => t.cost_usd > 0)
        .sort((a, b) => (b.created_at || '').localeCompare(a.created_at || ''));

    _costTaskCache = rows;

    if (rows.length === 0) {
        container.innerHTML = '<div class="cost-empty">No task data yet</div>';
        return;
    }

    const statusIcon = (status) => {
        if (status === 'completed') return '<span class="material-icons board-task-status-icon completed" style="font-size:14px">check_circle</span>';
        if (status === 'in_progress') return '<span class="task-spinner" title="In progress"></span>';
        if (status === 'skipped') return '<span class="material-icons board-task-status-icon skipped" style="font-size:14px">block</span>';
        return '<span class="material-icons board-task-status-icon pending" style="font-size:14px">radio_button_unchecked</span>';
    };

    let html = `<table class="cost-table">
        <thead><tr>
            <th style="width:24px"></th>
            <th>Task</th>
            <th>Agent</th>
            <th>Priority</th>
            <th class="cost-col-right">Input</th>
            <th class="cost-col-right">Output</th>
            <th class="cost-col-right">Cache Read</th>
            <th class="cost-col-right">Cache Write</th>
            <th class="cost-col-right">Cost</th>
        </tr></thead><tbody>`;

    for (const t of rows) {
        const priorityClass = 'board-task-priority-' + (t.priority || 'medium');
        html += `<tr onclick="_showCostTaskDetail(${t.id})" style="cursor:pointer">
            <td>${statusIcon(t.status)}</td>
            <td>${escapeHtml(t.title || '')}</td>
            <td class="cost-agent-name">${escapeHtml(t.assigned_to || '\u2014')}</td>
            <td><span class="board-task-priority ${priorityClass}">${escapeHtml(t.priority || 'medium')}</span></td>
            <td class="cost-col-right">${_formatTokens(t.input_tokens || 0)}</td>
            <td class="cost-col-right">${_formatTokens(t.output_tokens || 0)}</td>
            <td class="cost-col-right">${_formatTokens(t.cache_read_tokens || 0)}</td>
            <td class="cost-col-right">${_formatTokens(t.cache_write_tokens || 0)}</td>
            <td class="cost-col-right cost-cost-cell">${_formatCost(t.cost_usd || 0)}</td>
        </tr>`;
    }

    html += '</tbody></table>';
    container.innerHTML = html;
}

function _renderRequestLog(requests) {
    const container = document.getElementById('cost-request-log');
    if (!container) return;

    if (requests.length === 0) {
        container.innerHTML = '<div class="cost-empty">No requests yet</div>';
        return;
    }

    let html = `<table class="cost-table">
        <thead><tr>
            <th>Model</th>
            <th>Agent</th>
            <th class="cost-col-right">Input</th>
            <th class="cost-col-right">Output</th>
            <th class="cost-col-right">Cache R</th>
            <th class="cost-col-right">Cache W</th>
            <th class="cost-col-right">Cost</th>
            <th class="cost-col-right">Latency</th>
            <th>Status</th>
            <th>Time</th>
        </tr></thead><tbody>`;

    for (const r of requests) {
        const session = _findSession(r.session_id);
        const agentName = r.display_name || (session && session.display_name) || r.agent_name || '\u2014';
        const statusClass = r.status === 'completed' ? 'cost-status-ok'
            : r.status === 'error' ? 'cost-status-error'
            : 'cost-status-pending';
        html += `<tr>
            <td class="cost-model-name">${escapeHtml(r.model_used)}</td>
            <td class="cost-agent-name">${escapeHtml(agentName)}</td>
            <td class="cost-col-right">${_formatTokens(r.input_tokens)}</td>
            <td class="cost-col-right">${_formatTokens(r.output_tokens)}</td>
            <td class="cost-col-right">${_formatTokens(r.cache_read_tokens || 0)}</td>
            <td class="cost-col-right">${_formatTokens(r.cache_write_tokens || 0)}</td>
            <td class="cost-col-right cost-cost-cell">${_formatCost(r.cost_usd)}</td>
            <td class="cost-col-right">${_formatLatency(r.latency_ms)}</td>
            <td><span class="cost-status ${statusClass}">${escapeHtml(r.status)}</span></td>
            <td class="cost-date">${_formatDate(r.started_at)}</td>
        </tr>`;
    }

    html += '</tbody></table>';
    container.innerHTML = html;
}
