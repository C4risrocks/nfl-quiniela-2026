// NFL Quiniela 2026 Client Scripts

document.addEventListener('DOMContentLoaded', () => {
    // Listen for HTMX response events to show toast notifications
    document.body.addEventListener('htmx:afterRequest', (event) => {
        if (event.detail.successful) {
            // Pick saved indicator
            if (event.detail.pathInfo && event.detail.pathInfo.requestPath.includes('/picks/save')) {
                window.dispatchEvent(new CustomEvent('show-toast', {
                    detail: { message: '¡Pronóstico guardado con éxito!', type: 'success' }
                }));
            }
        } else {
            window.dispatchEvent(new CustomEvent('show-toast', {
                detail: { message: 'Error al procesar la solicitud', type: 'error' }
            }));
        }
    });
});
