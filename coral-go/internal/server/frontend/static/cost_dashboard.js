/* Cost Dashboard — token usage and cost tracking UI */

import { showView, escapeHtml } from './utils.js';

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

function _getSinceParam() {
    const range = document.getElementById('cost-time-range');
    if (!range) return '';
    const val = range.value;
    if (val === 'all') return '';
    const now = new Date();
    switch (val) {
        case '1h': now.setHours(now.getHours() - 1); break;
        case '24h': now.setHours(now.getHours() - 24); break;
        case '7d': now.setDate(now.getDate() - 7); break;
        case '30d': now.setDate(now.getDate() - 30); break;
    }
    return now.toISOString();
}

export function showCostDashboard() {
    showView('cost-dashboard-view');
    _refreshCostDashboard();
}

export async function _refreshCostDashboard() {
    const since = _getSinceParam();
    const qs = since ? `?since=${encodeURIComponent(since)}` : '';

    // Fetch summary and per-session data in parallel
    const [summaryResp, listResp] = await Promise.all([
        fetch(`/api/token-usage/summary${qs}`).catch(() => null),
        fetch(`/api/token-usage${qs}`).catch(() => null),
    ]);

    // Render summary cards
    if (summaryResp && summaryResp.ok) {
        const data = await summaryResp.json();
        const t = data.totals || {};
        _setText('cost-total-cost', _formatCost(t.cost_usd || 0));
        _setText('cost-total-tokens', _formatTokens(t.total_tokens || 0));
        _setText('cost-input-tokens', _formatTokens(t.input_tokens || 0));
        _setText('cost-output-tokens', _formatTokens(t.output_tokens || 0));
        _setText('cost-num-sessions', String(t.num_sessions || 0));
        _renderAgentTypeTable(data.by_agent_type || []);
    }

    // Render per-session table
    if (listResp && listResp.ok) {
        const data = await listResp.json();
        _renderSessionTable(data.records || []);
    }
}

export function _costTimeRangeChanged() {
    _refreshCostDashboard();
}

function _setText(id, text) {
    const el = document.getElementById(id);
    if (el) el.textContent = text;
}

function _renderAgentTypeTable(rows) {
    const container = document.getElementById('cost-by-agent-type');
    if (!container) return;

    if (rows.length === 0) {
        container.innerHTML = '<div class="cost-empty">No usage data yet</div>';
        return;
    }

    let html = `<table class="cost-table">
        <thead><tr>
            <th>Agent Type</th>
            <th class="cost-col-right">Sessions</th>
            <th class="cost-col-right">Input</th>
            <th class="cost-col-right">Output</th>
            <th class="cost-col-right">Total Tokens</th>
            <th class="cost-col-right">Cost</th>
        </tr></thead><tbody>`;

    for (const r of rows) {
        html += `<tr>
            <td><span class="cost-agent-badge cost-agent-${escapeHtml(r.agent_type)}">${escapeHtml(r.agent_type)}</span></td>
            <td class="cost-col-right">${r.num_sessions}</td>
            <td class="cost-col-right">${_formatTokens(r.input_tokens)}</td>
            <td class="cost-col-right">${_formatTokens(r.output_tokens)}</td>
            <td class="cost-col-right">${_formatTokens(r.total_tokens)}</td>
            <td class="cost-col-right cost-cost-cell">${_formatCost(r.cost_usd)}</td>
        </tr>`;
    }

    html += '</tbody></table>';
    container.innerHTML = html;
}

function _renderSessionTable(records) {
    const container = document.getElementById('cost-per-session');
    if (!container) return;

    if (records.length === 0) {
        container.innerHTML = '<div class="cost-empty">No usage data yet</div>';
        return;
    }

    let html = `<table class="cost-table">
        <thead><tr>
            <th>Agent</th>
            <th>Type</th>
            <th>Board</th>
            <th class="cost-col-right">Input</th>
            <th class="cost-col-right">Output</th>
            <th class="cost-col-right">Total</th>
            <th class="cost-col-right">Turns</th>
            <th class="cost-col-right">Cost</th>
            <th>Recorded</th>
        </tr></thead><tbody>`;

    for (const r of records) {
        const board = r.board_name || '\u2014';
        const recorded = _formatDate(r.recorded_at);
        html += `<tr>
            <td class="cost-agent-name">${escapeHtml(r.agent_name)}</td>
            <td><span class="cost-agent-badge cost-agent-${escapeHtml(r.agent_type)}">${escapeHtml(r.agent_type)}</span></td>
            <td class="cost-board-name">${escapeHtml(board)}</td>
            <td class="cost-col-right">${_formatTokens(r.input_tokens)}</td>
            <td class="cost-col-right">${_formatTokens(r.output_tokens)}</td>
            <td class="cost-col-right">${_formatTokens(r.total_tokens)}</td>
            <td class="cost-col-right">${r.num_turns}</td>
            <td class="cost-col-right cost-cost-cell">${_formatCost(r.cost_usd)}</td>
            <td class="cost-date">${recorded}</td>
        </tr>`;
    }

    html += '</tbody></table>';
    container.innerHTML = html;
}

function _formatDate(isoStr) {
    if (!isoStr) return '\u2014';
    try {
        const d = new Date(isoStr);
        return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
            + ' ' + d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
    } catch { return isoStr; }
}
