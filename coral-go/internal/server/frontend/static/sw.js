/* Coral Service Worker — caches app shell for fast loads */

const CACHE_NAME = 'coral-v1';
const SHELL_ASSETS = [
    '/',
    '/static/style.css',
    '/static/favicon.png',
    '/static/coral.png',
];

self.addEventListener('install', (event) => {
    event.waitUntil(
        caches.open(CACHE_NAME).then((cache) => cache.addAll(SHELL_ASSETS))
    );
    self.skipWaiting();
});

self.addEventListener('activate', (event) => {
    event.waitUntil(
        caches.keys().then((names) =>
            Promise.all(names.filter((n) => n !== CACHE_NAME).map((n) => caches.delete(n)))
        )
    );
    self.clients.claim();
});

self.addEventListener('fetch', (event) => {
    const url = new URL(event.request.url);

    // API and WebSocket requests: always go to network
    if (url.pathname.startsWith('/api/') || url.pathname.startsWith('/ws')) {
        return;
    }

    // Static assets: cache-first
    if (url.pathname.startsWith('/static/')) {
        event.respondWith(
            caches.match(event.request).then((cached) => cached || fetch(event.request))
        );
        return;
    }

    // HTML pages: network-first (fall back to cache for offline shell)
    event.respondWith(
        fetch(event.request).catch(() => caches.match('/'))
    );
});
