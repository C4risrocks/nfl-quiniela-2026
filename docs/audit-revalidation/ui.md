# Revalidación UI/UX

Firefox mediante Playwright, copia local, datos ficticios. Evaluación independiente iniciada por agente visual; consolidación del coordinador desde JSON/logs al interrumpirse por límite de capacidad. No se consultó detector para emitir el juicio visual.

- A01 resuelto en escenarios con históricos cero/nulos/no-cero:200, footer y contenedor readiness presentes, 17 cards; sin error de plantilla. Render buffer implementado. Los errores internos aún se muestran en respuestas de error del renderer; sanitización pendiente.
- A07 resuelto: labels enlazados, texto14px. Autocomplete añadido por fuente, pero DOM devolvió atributo vacío durante prueba; no se acredita autofill real.
- A09 parcial: dialog con nombre y aria-modal, Escape cierra y devuelve foco; apertura con Enter deja foco afuera y Tab pasa a Matriz detrás del diálogo. ui-runtime.json.
- A10 parcial: status/aria-live y sin truncado; mensaje visible desaparece tras4s. Sin lector de pantalla real.
- A11 resuelto para navegación con servidor detenido: service worker entrega pantalla Sin Conexión y botón reintentar, ui-offline.json. Persisten límites del fallback de recursos no HTML y dependencia de estilos externos; no acredita offline completo de todos los assets.
- A13 parcial: selector live cabe a360/390/768/1366/1440; densidad y metadatos pequeños persisten. Zoom nativo intentado con atajos no cambió métricas, por tanto no verificado. Captura zoom200 usa CSS, no zoom nativo.

Reduced motion reduce animaciones a0.00001s una iteración, no una alternativa diseñada. ShiftTab entre extremos del diálogo funciona, pero no corrige foco inicial. Evidencias completas ui-runtime.json, ui-extra.json, ui-offline.json, logs y capturas. Sin dispositivo físico, lector de pantalla, barrido completo de todos los estados ni certificación WCAG.
