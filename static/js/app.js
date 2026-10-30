// Shared request feedback. Status text is persistent; toasts are supplementary.
(() => {
    let revision = 0;
    function setPickStatus(message, state) {
        const status = document.getElementById('picks-save-status');
        if (!status) return;
        status.textContent = message;
        status.dataset.state = state;
        status.classList.toggle('text-rose-300', state === 'error');
        status.classList.toggle('text-emerald-300', state === 'saved');
        status.classList.toggle('text-zinc-300', state !== 'error' && state !== 'saved');
    }
    window.markPicksDirty = () => {
        revision++;
        setPickStatus('Cambios sin enviar. Pulsa Enviar para guardarlos.', 'unsaved');
    };
    function serverMessage(xhr) {
        try {
            const trigger = JSON.parse(xhr.getResponseHeader('HX-Trigger') || '{}');
            return trigger['show-toast']?.message || '';
        } catch (_) { return ''; }
    }
    function failureMessage(status, retryAfter) {
        if (status === 429) {
            const seconds = Number.parseInt(retryAfter, 10);
            return Number.isFinite(seconds) && seconds > 0
                ? `No se guardaron tus cambios. Espera ${seconds} segundos y vuelve a enviar.`
                : 'No se guardaron tus cambios. Espera un momento y vuelve a enviar.';
        }
        if (status === 401) return 'Tu sesión terminó. Inicia sesión de nuevo antes de guardar.';
        if (status === 403) return 'No se guardaron tus cambios: la solicitud fue rechazada. Revisa tu sesión y vuelve a intentarlo.';
        if (!status) return 'No se confirmó el guardado. Revisa tu conexión y vuelve a enviar; tus selecciones siguen en esta página.';
        return 'No se confirmó el guardado. Tus selecciones siguen en esta página; vuelve a enviar.';
    }
    window.submitPicks = async form => {
        if (!form || form.dataset.saving === 'true' || !form.reportValidity()) return;
        const body = new URLSearchParams(new FormData(form));
        const submittedRevision = revision;
        form.dataset.saving = 'true';
        form.setAttribute('aria-busy', 'true');
        const buttons = [...document.querySelectorAll('[data-picks-submit]')];
        const disabled = buttons.map(button => button.disabled);
        buttons.forEach(button => button.disabled = true);
        setPickStatus('Guardando pronósticos…', 'saving');
        try {
            const response = await fetch(form.action, {
                method: 'POST', body, credentials: 'same-origin', headers: { 'HX-Request': 'true' }
            });
            const url = new URL(response.headers.get('HX-Redirect') || response.url, location.origin);
            if (!response.ok) {
                setPickStatus(failureMessage(response.status, response.headers.get('Retry-After')), 'error');
                return;
            }
            if (url.origin !== location.origin || url.pathname !== '/picks' || url.searchParams.get('saved') !== '1') {
                setPickStatus('No se confirmó el guardado. Revisa que tu sesión y correo estén verificados antes de volver a enviar.', 'error');
                return;
            }
            if (revision !== submittedRevision) {
                setPickStatus('Se guardaron los pronósticos enviados. Tienes cambios posteriores sin enviar.', 'unsaved');
                return;
            }
            const missing = Number.parseInt(url.searchParams.get('missing') || '0', 10);
            setPickStatus(missing > 0
                ? `Pronósticos guardados. Quedan ${missing} partidos sin completar.`
                : 'Pronósticos guardados correctamente.', 'saved');
            // Refresh server-derived readiness and counts only after confirmed success.
            // A rejected request or edits made while saving keep this draft intact.
            location.assign(url.href);
        } catch (_) {
            setPickStatus(failureMessage(0), 'error');
        } finally {
            form.dataset.saving = 'false';
            form.removeAttribute('aria-busy');
            buttons.forEach((button, index) => button.disabled = disabled[index]);
        }
    };
    document.addEventListener('DOMContentLoaded', () => {
        const form = document.getElementById('picks-form');
        if (form) {
            form.addEventListener('input', window.markPicksDirty);
            form.addEventListener('change', window.markPicksDirty);
            form.addEventListener('submit', event => {
                event.preventDefault();
                window.submitPicks(form);
            });
        }
        document.body.addEventListener('htmx:afterRequest', event => {
            const detail = event.detail;
            const xhr = detail.xhr;
            const status = xhr?.status || 0;
            const message = xhr ? serverMessage(xhr) : '';
            const path = detail.pathInfo?.requestPath || '';
            const isPickSave = /^\/picks\/save(?:-score|-all)?(?:\?|$)/.test(path);
            if (isPickSave) {
                setPickStatus(detail.successful ? 'Pronóstico guardado correctamente.' : (message || failureMessage(status, xhr?.getResponseHeader('Retry-After'))), detail.successful ? 'saved' : 'error');
            }
            // HX-Trigger already supplies a useful message for rejected requests.
            // Never replace authentication, CSRF or rate-limit feedback with a generic toast.
            if (detail.successful || message || [401, 403, 429].includes(status)) return;
            window.dispatchEvent(new CustomEvent('show-toast', {
                detail: { message: 'No se pudo completar la solicitud. Vuelve a intentarlo.', type: 'error' }
            }));
        });
    });
})();
