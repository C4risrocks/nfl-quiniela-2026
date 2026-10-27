// Service Worker for NFL Quiniela 2026 PWA
const CACHE_NAME = 'quiniela-pwa-v1';
const STATIC_ASSETS = [
  '/static/manifest.json',
  '/static/icons/icon.svg'
];

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => {
      return cache.addAll(STATIC_ASSETS);
    })
  );
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) => {
      return Promise.all(
        keys.filter((key) => key !== CACHE_NAME).map((key) => caches.delete(key))
      );
    })
  );
  self.clients.claim();
});

// Network-first strategy with cache fallback
self.addEventListener('fetch', (event) => {
  // Only handle GET requests
  if (event.request.method !== 'GET') return;
  // Ignore SSE events and API sync
  if (event.request.url.includes('/events/live')) return;

  event.respondWith(
    fetch(event.request).catch(() => {
      return caches.match(event.request);
    })
  );
});

// ==============================================================================
// Web Push Notifications (RFC 8292 / VAPID)
// ==============================================================================

self.addEventListener('push', (event) => {
  let data = {};
  if (event.data) {
    try {
      data = event.data.json();
    } catch (e) {
      data = {
        title: '🏈 NFL Quiniela 2026',
        body: event.data.text()
      };
    }
  }

  const title = data.title || '🏈 NFL Quiniela 2026';
  const options = {
    body: data.body || 'Tienes una nueva actualización en tu Quiniela NFL.',
    icon: data.icon || '/static/icons/icon.svg',
    badge: data.badge || '/static/icons/icon.svg',
    vibrate: [150, 50, 150],
    tag: data.tag || 'quiniela-notification',
    renotify: true,
    requireInteraction: false,
    data: {
      url: data.url || '/picks',
      timestamp: Date.now()
    }
  };

  event.waitUntil(
    self.registration.showNotification(title, options)
  );
});

// Handle push notification clicks
self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const targetUrl = (event.notification.data && event.notification.data.url) ? event.notification.data.url : '/picks';

  event.waitUntil(
    clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clientList) => {
      // If a quiniela tab is already open, navigate and focus it
      for (let i = 0; i < clientList.length; i++) {
        const client = clientList[i];
        if (client.url && 'focus' in client) {
          if ('navigate' in client) {
            client.navigate(targetUrl);
          }
          return client.focus();
        }
      }
      // Otherwise open a new window
      if (clients.openWindow) {
        return clients.openWindow(targetUrl);
      }
    })
  );
});

// Automatically handle subscription rotation by browser
self.addEventListener('pushsubscriptionchange', (event) => {
  event.waitUntil(
    fetch('/api/push/vapid-public-key')
      .then((res) => res.json())
      .then((data) => {
        if (!data.publicKey) return;
        return self.registration.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: data.publicKey
        });
      })
      .then((newSub) => {
        if (!newSub) return;
        return fetch('/api/push/subscribe', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(newSub)
        });
      })
      .catch((err) => {
        console.warn('[SW] pushsubscriptionchange renewal failed:', err);
      })
  );
});

