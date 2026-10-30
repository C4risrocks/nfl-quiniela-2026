// Service Worker for NFL Quiniela 2026 PWA
const CACHE_NAME = 'quiniela-pwa-v2';
const OFFLINE_URL = '/offline';
const STATIC_ASSETS = [
  '/static/manifest.json',
  '/static/icons/icon.svg',
  '/offline'
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

// Network-first strategy with dedicated offline fallback for navigation (A11)
self.addEventListener('fetch', (event) => {
  // Only handle GET requests
  if (event.request.method !== 'GET') return;
  // Ignore SSE events and API sync
  if (event.request.url.includes('/events/live')) return;

  // Handle HTML navigation requests with dedicated offline screen fallback
  if (event.request.mode === 'navigate') {
    event.respondWith(
      fetch(event.request).catch(async () => {
        const cache = await caches.open(CACHE_NAME);
        const cachedOffline = await cache.match(OFFLINE_URL);
        if (cachedOffline) {
          return cachedOffline;
        }
        return new Response(
          '<!DOCTYPE html><html lang="es"><head><meta charset="utf-8"><title>Sin conexión</title></head><body style="background:#09090b;color:#fff;font-family:sans-serif;text-align:center;padding:50px;"><h1>🏈 Sin Conexión</h1><p>Conéctate a Internet para ingresar tus pronósticos.</p><button onclick="window.location.reload()" style="background:#fff;color:#000;padding:10px 20px;border-radius:8px;font-weight:bold;cursor:pointer;">Reintentar</button></body></html>',
          { headers: { 'Content-Type': 'text/html; charset=utf-8' } }
        );
      })
    );
    return;
  }

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

