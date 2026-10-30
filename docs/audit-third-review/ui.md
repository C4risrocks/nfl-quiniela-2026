# Tercera auditoría independiente de UI y UX

Snapshot objetivo72dd947; revisión del2026-10-30, comparación con hallazgos originales y segunda revalidación. Entorno propio aislado `/tmp/nfl-ui-third.Gm56kB`, puerto18110, SQLite ficticio y Firefox Playwright. No se modificó código de aplicación.

## Alcance y método

Revalidar A01/A07/A09/A10/A11/A13; foco por teclado al abrir/cerrar modal HTMX, feedback, móvil, offline y presentación de errores CSRF/rate-limit. Se usa un admin ficticio y datos de partidos con marcadores0/14,17/14 y nulos. El servidor temporal tiene sincronización periódica y recordatorios desactivados; no envía notificaciones.

La revisión se basa en fuente actual y navegador; cobertura consultada para13archivos usados: el índice previo tiene metadatos desactualizados/huecos y no se presume exhaustivo. Se verificaron las fuentes citadas directamente.

## Matriz de revalidación

| ID | Estado actual | Evidencia y límites |
|---|---|---|
| A01 | Corregido el defecto original | `/picks?week=18` devuelve200 con footer, contenedores y17tarjetas, sin error técnico. Fixture H2H0/14,17/14 y nulos. `game_card.html:438,440` compara escalares; `handlers/renderer.go:235-247` usa buffer. Residual: mensajes detallados de plantilla aún salen mediante http.Error; no se interpreta esto como documento truncado. |
| A07 | Corregida asociación de etiquetas | Login y registro devuelven labels asociados por input y nombres útiles; estilos computados14px. Fuente `login.html:33-44`/`register.html:44-58` incluye autocomplete. No se verificó lector de pantalla ni gestor de contraseñas; el harnessFirefox muestra propiedad autocomplete vacía, por lo que no se afirma funcionamiento de autofill. |
| A09 | Corregido en preparación; otros ciclos pendientes | Abrir preparación con Enter mueve foco dentro; siguienteTab permanece en diálogo; Escape lo elimina y devuelve foco al activador. Nombre, role=dialog y aria-modal correctos. Mejora frente a segunda auditoría, donde Tab escapaba detrás del overlay. No se validó cada modal ni actualización periódica de estadísticas. |
| A10 | Parcial, mejorado | role=status/aria-live=polite, texto sin truncate, botón Cerrar notificación funcional tras transición. Un éxito sigue ocultándose a5.5s; fuente base.html:654-672 configura5s éxito/8s error. No existe confirmación persistente añadida ni lector de pantalla probado. Los toasts consecutivos ahora cancelan temporizador anterior. |
| A11 | Corregido el fallback de navegación | Cache contiene/offline. Con servidor18110 detenido, URL nunca visitada devuelve200 desdeSW, h1«Sin Conexión a Internet» y Reintentar. Captura offline.png legible. No se probó instalación fría sin conexión ni recuperación mediante el botón tras reiniciar servidor. |
| A13 | Parcial | Selectorlive cabe en360/390/768/1366/1440: bordes derechos327/357/653.57/1270/1307. Inputs14px. Permanece densidad de controles/metadatos pequeños en pronósticos; esta es valoración de diseño, no infracción por tamaño aislado. |

## Errores de seguridad y feedback

**Rate-limit confirmado en navegador.** Con límite de autenticación temporal12/min, los últimos intentos ficticios devuelven429 y Retry-After5. Una petición real HTMX muestra «Demasiados intentos de acceso. Espera5segundos.» mediante toast visible. Cerrar notificación la oculta tras finalizar transición; la lectura inmediatamente después del click aún veía la animación, no un fallo de cierre. Captura rate-limit-toast.png.

**CSRF confirmado a nivelHTTP; entrega visual no verificada.** UnPOST con Origin no autorizado y HX-Request=true devuelve403 y HX-Trigger con «Solicitud bloqueada: origen no autorizado (CSRF).». El primer intento de sustituir Origin mediante route dePlaywright no interceptó elfetch delServiceWorker y no generó toast; no se clasifica como fallo de protección. La pruebaHTTP posterior sí confirmó rechazo. ui-final-check.log conserva status, cuerpo y trigger, sin cookies/tokens. La captura csrf-toast.png pertenece al intento no concluyente y no demuestra rechazo visual.

## Riesgo residual de foco por actualización

`templates/layouts/base.html:966-975` enfoca el primer control después de **cualquier** swap cuyo target contenga modal. `game_stats_modal.html:9-13` refresca cada15s durante un partido en juego. Por inspección, esto puede reiniciar el foco durante la interacción de teclado con estadísticas. Es un riesgo específico que requiere reproducir con partido en vivo; no se presenta como regresión dinámica confirmada. Limitar el enfoque inicial a apertura y conservar el elemento/tab activo en refrescos.

## Evidencia, alcance y limpieza

- ui-runtime.json/log: labels, render completo, teclado de preparación, toast y cinco anchos.
- ui-errors.json/log: rate429 y feedback; primer intentoCSRF no concluyente.
- ui-final-check.json/log:403CSRF real porHTTP, trigger, cierre de toast y cacheoffline.
- ui-offline.json/log: navegación nueva con servidor detenido.
- Capturas picks-mobile.png, readiness-mobile.png, rate-limit-toast.png y offline.png.

Se inspeccionó el comportamiento renderizado enFirefox con viewport móvil; los cinco anchos se midieron en DOM. No se ejecutó detector ni se consumieron resultados de otros revisores: evaluación independiente. No se probaron lector de pantalla, dispositivo físico, zoom nativo ni todas las rutas/estados. Los intentos previos de zoom nativo mediante atajos no modificaron viewport/DPR en este harness; no se sustituye esa validación por afirmaciones basadas enCSSzoom. No se observaron nuevas regresiones dinámicas en las superficies probadas; el riesgo de foco periódico anterior permanece pendiente.

Firefox third-ui cerrado y servidor18110 detenido. Históricos de docs/audit-revalidation restaurados exactosHEAD; solo se añaden los artefactos de esta tercera revisión. La copia temporal conserva datos ficticios para reproducción y no contiene credenciales privadas.
