# Evaluación independiente de UI y UX — Assessment A

Fecha local: 2026-10-29. Servicio público; participante nuevo y usuario móvil como personas principales. Evaluación independiente del detector: no se consultaron resultados de Assessment B.

## Evidencia y límites

Se revisaron directamente login, registro, layout global, pronósticos, modal de estadísticas, modal de preparación, CSS y JavaScript global. Se consultó cobertura del índice para cada archivo citado: hay cobertura parcial y cambios de metadatos en layout/game_card, por lo que los hallazgos se fundamentan en fuente y navegador, no en supuesta exhaustividad del grafo.

Navegación real mediante Playwright CLI y Firefox, localhost:18089, datos aislados y administrador ficticio. Login inspeccionado visualmente a 390×844 y 1366×768; pronósticos a 390×844. No se usó dispositivo físico ni lector de pantalla. No se afirma validación de gestos táctiles, Safari ni zoom. Chrome inicialmente no arrancó por falta del canal Google Chrome; Firefox sí funcionó. La sesión auditux se cerró.

Capturas: `login-mobile.png`, `login-desktop.png`, `picks-mobile.png`. `readiness-mobile.png` documenta la página después del intento de apertura fallido, no un modal abierto.

## Veredicto visual

La identidad oscura y compacta es consistente en login y navegación; logos, marcadores y etiquetas de estado identifican claramente el dominio NFL en la página de pronósticos. El login tiene una jerarquía simple y un CTA visible. En móvil la densidad aumenta: el primer partido comienza aproximadamente a 558 px de altura, después de controles de jornada, preparación, progreso, plataformas y modos. El usuario debe interpretar numerosas herramientas antes de realizar su tarea principal.

## Hallazgos prioritarios

### UX-01 — P1: fallo de plantilla deja pronósticos incompletos

**Confirmado en navegador.** Tras iniciar sesión se muestra Semana 6. El HTML renderizado termina dentro del primer partido con `error calling gt: invalid type for comparison`. El DOM no contiene `readiness-modal-container`, footer ni navegación inferior. Pulsar `0/1 listos 0%` provoca `htmx:targetError`; no es una segunda causa independiente.

Fuente: `templates/partials/game_card.html:438` y `:440`, comparaciones de `.HomeScore` y `.AwayScore` en histórico H2H. Aunque el bloque se oculta mediante Alpine, el servidor lo evalúa. Impacto: la página se interrumpe y desaparecen controles y contenido posterior; la interfaz parece parcialmente operativa.

Reproducción: login en la instancia aislada, abrir `/picks` con históricos H2H que contengan marcadores y consultar `document.querySelector('main').textContent`/contenedores. Recomendación: normalizar/desreferenciar marcadores antes de compararlos y renderizar a buffer antes de escribir la respuesta, con prueba de partidos e históricos que tengan valores nulos, cero y resultados válidos. Esfuerzo M.

### UX-02 — P1: campos de acceso y registro sin asociación programática con etiquetas

`templates/pages/login.html:33-44`; `templates/pages/register.html:44-58`. Los labels no tienen `for` y los inputs no tienen `id`, ni están anidados en su label. En Firefox ambos campos de login devolvieron `labels.length = 0`; el árbol accesible muestra como nombres los placeholders `tu_usuario` y `••••••••`, no las etiquetas visibles. El placeholder del password no identifica el propósito.

Impacto: dificultad para lectores de pantalla y reconocimiento por voz; pulsar la etiqueta no enfoca su campo. Relacionado con identificación de controles y relaciones de formulario. Añadir id/for y autocomplete apropiado (username, current-password, email, new-password); enlazar mensajes de validación al campo. Esfuerzo S.

### UX-03 — P1: modales sin semántica ni gestión explícita de foco

Verificado en fuente, pendiente de ejecución dinámica porque UX-01 impide abrir preparación en el escenario observado. `templates/partials/game_stats_modal.html:1-42`, `templates/partials/picks_readiness_modal.html:1-5` y controlador en `templates/layouts/base.html:833-860`: Escape y cierre por fondo existen, pero esos modales no declaran dialog/aria-modal/aria-labelledby, ni trampa de foco, enfoque inicial o retorno al activador. El controlador elimina HTML y desbloquea scroll; no restituye foco.

Impacto esperado: recorrido de teclado por contenido tapado y pérdida de contexto al abrir/cerrar; requiere confirmar comportamiento con escenarios no afectados por UX-01. Añadir patrón de diálogo accesible completo y preservar foco al refrescar estadísticas. Esfuerzo M. No se presenta como fallo dinámico reproducido.

### UX-04 — P2: confirmaciones efímeras no anunciadas a tecnologías de apoyo

`templates/layouts/base.html:652-669`: toast desaparece a los tres segundos, texto truncado y sin región status/aria-live. `static/js/app.js:4-17` envía confirmaciones y errores a ese mecanismo. La ausencia de región está confirmada por fuente; el anuncio en lector de pantalla no fue probado.

Impacto: quien no vea el toast puede no saber si se guardó su pronóstico; errores largos pierden información. Añadir región de estado y estados persistentes en la acción; usar mensajes de error recuperables con contexto. Esfuerzo S.

### UX-05 — P2: legibilidad y densidad en móvil

Login usa inputs de 12 px y labels de 11 px (`login.html:33-44`), confirmado con computed styles. Pronósticos muestra metadatos de 9–10 px y varias filas de controles antes de los partidos; captura `picks-mobile.png`. El carrusel de acciones corta visualmente el asistente a la derecha. No se afirma que el desplazamiento esté roto.

Impacto: mayor esfuerzo de lectura y descubrimiento para usuarios nuevos o visión reducida; la tarea principal queda subordinada a herramientas auxiliares. Priorizar jornada/progreso/guardado y agrupar herramientas secundarias; elevar el tamaño de texto operativo y comprobar 200 % de zoom y 360 px. Esfuerzo M. Esta es una valoración de diseño, no un incumplimiento WCAG por tamaño de fuente solamente.

## Heurísticas de Nielsen

Escala 0–4, evaluación provisional de las superficies observadas; no constituye puntuación integral del producto.

| Heurística | Resultado | Evidencia |
|---|---|---|
| Visibilidad del estado | 2 | Progreso y bloqueos visibles; página puede quedar truncada y toast efímero. |
| Correspondencia con mundo real | 3 | Equipos, semana y patada inicial claros; IA/Elo/Beta requieren aprendizaje. |
| Control y libertad | 2 | Escape existe en fuente; foco y recuperación necesitan completar. |
| Consistencia | 3 | Identidad oscura, patrones y navegación coherentes en login/picks. |
| Prevención de errores | n/a | No se probó guardado ni acciones destructivas. |
| Reconocer antes que recordar | 2 | Herramientas visibles, pero numerosas y algunas fuera del ancho inicial. |
| Flexibilidad y eficiencia | 3 | Modos y llenado asistido presentes; no se validó aplicación de cambios. |
| Diseño minimalista | 2 | Login claro; controles de picks compiten con tarea central. |
| Reconocer y recuperar errores | 1 | Error técnico incrustado en HTML; fallo de modal sin recuperación visible. |
| Ayuda y documentación | 2 | Reglamento accesible en navegación; no se validó exhaustivamente su contenido. |

## Fortalezas a preservar

- CTA de acceso contrastado y enlace a recuperación próximo a contraseña.
- Explicación explícita de bloqueo por partido y etiquetas de estado en pronósticos.
- Diseño contempla navegación inferior con safe-area, scroll de modales y cierre por Escape; mejorar accesibilidad sobre esa base.

El mayor valle de la experiencia observada llega después de un login correcto: la página aparenta cargar, pero el primer histórico interrumpe el resto. Resolver UX-01 debe preceder a evaluar guardado, preparación y foco en el flujo completo.

## Ampliación de cobertura dinámica

Resultados estructurados completos en `browser-sweep.json` (70 filas) y `browser-interactions.json`; logs correspondientes conservan la ejecución. Barrido de `/login`, `/register`, `/forgot-password`, `/rules`, `/leaderboard`, `/teams`, `/playoffs`, `/stats`, `/live` como visitante y `/picks`, `/profile`, `/admin`, `/picks/advisor`, `/picks/matrix` como administrador, a 360, 390, 768, 1366 y 1440 px de ancho, altura 900. Una fila —picks a 360— fue interrumpida por una navegación de diagnóstico del auditor y se clasifica como no concluyente; las otras 69 completaron. Picks a 390/768/1366/1440 reproduce el error de plantilla. Ninguna de las 69 reportó scroll global horizontal; esto no prueba ausencia de recortes, porque el layout oculta overflow global.

Se capturaron primeras vistas de las rutas a 360 y 1366 px (excepto picks360, interrumpido); se inspeccionaron visualmente login, reglas, posiciones, equipos, playoffs, analítica, vivo, perfil, administración, asesor y matriz en móvil y superficies representativas en escritorio. No se recorrieron todos los estados internos ni todas las pestañas de esas páginas. Capturas `public-*` y `auth-*` corresponden a datos ficticios locales y no son comparaciones visuales automatizadas.

El selector de jornada de `/live` a 360 px tiene borde derecho en x=364.52 px, fuera del viewport. La captura `public-live-360.png` muestra recorte a la derecha. Se incorpora como evidencia concreta de UX-05: ajustar anchura mínima/flex del selector y contenedor; la ausencia de scrollbar global no elimina este defecto.

Teclado login confirmado desde autofocus: usuario → recuperación → contraseña → Continuar → registro. No hubo trampa en ese tramo; no se probó orden completo global ni lector de pantalla.

Con `prefers-reduced-motion: reduce` emulado, tres animaciones pulse de indicadores de vivo permanecen activas. Observación de mejora dentro de UX-05, sin afirmar parpadeo peligroso ni bloqueo de tarea. Captura `login-css-zoom200.png` usa CSS zoom=2 (no zoom nativo del navegador): el submit sigue dentro del ancho, la navegación inferior queda muy densa y requiere mayor recorrido vertical. La validación de zoom nativo y reflow con tecnologías de apoyo permanece pendiente.

Como participante verificado sin beta: `/picks` y `/profile` abren; `/admin` redirige a `/picks`; asesor y matriz muestran contenido de bloqueo beta (el 200 registrado es respuesta final después de redirección o página informativa, no prueba de acceso administrativo). Admin sí accede a ambas herramientas. No se probó en navegador el rol sin verificar ni se modificaron flags de la base; su redirección y pantalla de recuperación se revisaron en fuente. No se enviaron correos ni push.

El barrido observó en consola la advertencia de Alpine de que `x-collapse` requiere el plugin Collapse. No se verificó una interacción fallida por esta causa, por lo que no se cuenta como hallazgo independiente.

La sesión Firefox `auditux` está cerrada. No queda navegador de esta evaluación abierto.

Questions skipped: el usuario autorizó una auditoría con informe, sin solicitar cambios de diseño; las decisiones de corrección se documentan para una fase posterior.
