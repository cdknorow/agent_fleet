/* Lightweight hash-based router for back/forward navigation.
   Pushes state on view transitions, restores on popstate. */

import { state } from './state.js';

/** Push a view state to history. */
export function pushView(view, params = {}) {
    const hash = _buildHash(view, params);
    if (window.location.hash === hash) return; // avoid duplicate pushes
    history.pushState({ view, ...params }, '', hash);
}

/** Replace current view state (doesn't add to history stack). */
export function replaceView(view, params = {}) {
    const hash = _buildHash(view, params);
    history.replaceState({ view, ...params }, '', hash);
}

function _buildHash(view, params) {
    if (view === 'chat' && params.sessionId) return `#chat/${params.sessionId}`;
    if (view === 'board' && params.boardName) return `#board/${encodeURIComponent(params.boardName)}`;
    if (view === 'history' && params.sessionId) return `#history/${params.sessionId}`;
    return `#${view}`;
}

/** Parse a hash string into {view, params}. */
function _parseHash(hash) {
    if (!hash || hash === '#') return { view: 'agents', params: {} };
    const parts = hash.replace('#', '').split('/');
    const view = parts[0] || 'agents';
    const id = parts.slice(1).join('/');
    return {
        view,
        params: id ? { sessionId: decodeURIComponent(id), boardName: decodeURIComponent(id) } : {},
    };
}

/** Initialize the router — listen for popstate and handle initial hash. */
export function initRouter() {
    window.addEventListener('popstate', (e) => {
        const viewState = e.state || _parseHash(window.location.hash);
        _restoreView(viewState.view || 'agents', viewState);
    });

    // Set initial state without pushing
    if (!window.location.hash) {
        replaceView('agents');
    }
}

function _restoreView(view, params) {
    const isMobile = window.innerWidth <= 767;

    switch (view) {
        case 'agents':
            if (isMobile && window.switchMobileTab) {
                window.switchMobileTab('agents');
            }
            break;
        case 'chat':
            if (params.sessionId && window.selectLiveSession) {
                // selectLiveSession handles showing the session view
            } else if (isMobile && window.mobileBack) {
                window.mobileBack();
            }
            break;
        case 'board':
            if (isMobile && window.switchMobileTab) {
                window.switchMobileTab('board');
            }
            break;
        case 'history':
            if (params.sessionId && window.selectHistorySession) {
                window.selectHistorySession(params.sessionId);
            }
            break;
        default:
            // Unknown view — go to agents
            if (isMobile && window.switchMobileTab) {
                window.switchMobileTab('agents');
            }
    }
}
