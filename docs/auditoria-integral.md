# Auditoría integral — NFL Quiniela 2026

Fecha: 29 de octubre de 2026, America/Mexico_City. Snapshot: `a6dc4c98e9088640b6ee423effa9360c137e3de5`. Objetivo: servicio público. Entorno: copia temporal del commit, SQLite aislado y PostgreSQL 17 en contenedor desechable. No se modificó la aplicación ni se auditó el despliegue real.

## Dictamen ejecutivo

**Hay problemas que deben resolverse antes de considerar listo un lanzamiento público.** La página central de pronósticos puede truncarse por un error de plantilla; la alternativa PostgreSQL no inicializa los datos de negocio y el healthcheck público revela su cadena de conexión, incluida la contraseña. También se confirmaron pronósticos inválidos, sesiones que sobreviven al cambio de contraseña y peticiones push hacia loopback controladas por un usuario.

Un fallo de escritura inyectado además dejó puntos de pronóstico y tabla inconsistentes mientras el recálculo devolvía éxito.

Las pruebas existentes y el detector de carreras pasan. Esto acredita los escenarios cubiertos, pero no evita los defectos reproducidos aquí. No se asigna una puntuación global de calidad ni una certificación de accesibilidad: las mediciones visuales son parciales y no se inspeccionó infraestructura productiva.

El [inventario estático](audit-evidence/routes.md) enumera 86 rutas explícitas; el [método](audit-evidence/method.md) registra herramientas y aislamiento.

La evidencia visual independiente está en [UI/UX](audit-evidence/ui-ux.md), y el análisis técnico independiente en [backend y detector](audit-evidence/technical.md). El informe principal consolida causas y evita contar sus consecuencias como defectos separados.

## Hallazgos y prioridades

17 hallazgos consolidados: 9 P1 y 8 P2. Algunos se sustentan en inspección de código o juicio de diseño, señalados expresamente; no todos son explotaciones reproducidas. Los riesgos pendientes aparecen separados y no se suman a este total.

Prioridades: P0 incidente o bloqueo total sin alternativa; P1 riesgo alto o tarea esencial afectada; P2 impacto acotado o mejora necesaria; P3 mantenimiento menor. Esfuerzo S: cambio localizado; M: varios componentes/pruebas; L: trabajo transversal. Las estimaciones no son compromisos de tiempo.

### P1 — Corregir antes de apertura pública

**A01. Pronósticos se entrega incompleto con HTTP 200.** Confirmado por HTTP y Firefox. `templates/partials/game_card.html:438,440` compara marcadores mediante `gt` y falla con `invalid type for comparison` al renderizar históricos. `handlers/renderer.go:229-230` ya escribió parte del documento cuando intenta emitir el error. Desaparecen contenido posterior, contenedores de modales y navegación. Reproducción: login y `/picks` con los históricos presentes en el entorno; inspeccionar el final del HTML y pulsar preparación. Evidencia: [smoke HTTP](audit-evidence/http-smoke.txt), [captura](audit-evidence/picks-mobile.png). Normalizar valores opcionales antes de compararlos y renderizar en buffer; emitir un error genérico y registrar detalles del lado servidor. Probar nulos, cero e históricos terminados, y comprobar documento completo. Esfuerzo M.

**A02. PostgreSQL conecta, pero el repositorio usa SQL incompatible.** Confirmado en PostgreSQL 17 vacío. `db/repository.go:210-211,720,974-977` usa placeholders `?`, sin conversión a `$1` para pgx. El arranque registra errores SQL al crear administrador/equipos/temporada. El esquema existe, pero `teams` queda con cero filas. Reproducción: iniciar con `DB_TYPE=postgres` y una base vacía; consultar logs y `SELECT count(*) FROM teams`. Evidencia: [arranque](audit-evidence/postgres.txt), [conteos](audit-evidence/postgres-counts.txt). Introducir adaptación consistente de consultas y pruebas reales por dialecto; no anunciar compatibilidad completa hasta que CRUD, semillas y cálculos pasen. Esfuerzo L.

**A03. `/healthz` público expone contraseña PostgreSQL.** Confirmado dinámicamente. `db/db.go:33-36` guarda el DSN íntegro como DBPath; `main.go:221-250` lo serializa sin autenticación. Una visita obtiene host, usuario, contraseña y base, aunque acceder a la base desde Internet depende de su red. Reproducción: `GET /healthz` en la instancia PostgreSQL. Evidencia: el DSN figura en el log y el comportamiento se verificó en ejecución; solo se usaron credenciales ficticias; [respuesta redactada](audit-evidence/postgres-health.json). Eliminar DSN/paths de la respuesta pública y separar diagnóstico autenticado. Rotar credenciales si este comportamiento existe en producción. Esfuerzo S.

**A04. Secretos predeterminados y contraseña de administrador en logs.** Confirmado por código: `config/config.go:62-65` acepta valores conocidos; `main.go:372` imprime incluso una contraseña personalizada. `services/auth/auth.go:121-160` firma sesiones con ese secreto, por lo que conservar el predeterminado permite fabricar firmas válidas. El despliegue real no se comprobó. Rechazar configuraciones inseguras para producción y retirar contraseñas de logs. Probar rechazo de configuración y ausencia de secretos en salida. Esfuerzo S.

**A05. Cambiar contraseña no invalida sesiones anteriores.** Confirmado dinámicamente en una cuenta ficticia: cookie anterior al cambio siguió accediendo a `/profile` con HTTP 200. Fuente: tokens stateless de 30 días en `services/auth/auth.go:121-175`; actualización del hash en `db/repository.go:423-436`. Evidencia: [prueba](audit-evidence/session-revocation.txt). Introducir revocación/versionado de sesión para cambio y recuperación de contraseña; probar cookies previas, logout y expiración. Esfuerzo M.

**A06. Cookie sin atributo Secure.** Confirmado en respuesta de login; `services/auth/auth.go:175` fija `false`. Con un acceso HTTP al dominio puede enviarse la cookie sin cifrar; HTTPS/HSTS del sitio real no fue evaluado. Evidencia: [smoke HTTP](audit-evidence/http-smoke.txt). Establecer una política segura de cookies para HTTPS y probar atributos y expiración. Esfuerzo S.

**A07. Formularios de acceso y registro sin etiquetas asociadas.** Confirmado en fuente y DOM de Firefox (`labels.length=0` para login). `templates/pages/login.html:33-44` y `register.html:44-58` no enlazan label/input; el campo de contraseña queda identificado por puntos del placeholder. Impacto en lectura asistida y entrada por voz. Añadir id/for y autocomplete adecuado y asociar errores. Evidencia visual y detalle en [UI/UX](audit-evidence/ui-ux.md). Esfuerzo S.

**A14. Push permite peticiones del servidor a destinos locales elegidos por el usuario (SSRF).** Confirmado en harness aislado: Subscribe devuelve 200 con un endpoint de loopback, SendTest devuelve 200 con delivered=1 y el señuelo recibe POST. `handlers/push_handler.go:65-81` solo valida campos no vacíos; `services/notifications/webpush.go:76-93` usa directamente el endpoint. Se utilizaron claves válidas y un servidor httptest propio, sin sondear red interna real. Evidencia y harness: [evaluación técnica](audit-evidence/technical.md). Restringir esquema/destinos, direcciones privadas y loopback, resolución DNS y redirecciones; añadir límites y pruebas de salida. No se afirma lectura arbitraria de respuestas internas. Esfuerzo M.

**A16. El recálculo confirma éxito con puntos inconsistentes tras un fallo de escritura.** P1, confirmado con inyección de fallo controlada. `services/scoring/calculator.go:129-135,294-310` registra errores de escritura y continúa. Un trigger temporal rechazó UPDATE de points_earned: CalculateWeekScores devolvió nil, el pronóstico quedó en 0 y weekly_leaderboard en 17. Evidencia y harness: [resultado](audit-evidence/business-result.txt), [fuente](audit-evidence/business-repro.go.txt). Ejecutar la unidad de recálculo en transacción, propagar errores y publicar eventos solo tras commit. Probar rollback por fallo intermedio y reintento idempotente. Esfuerzo M.

### P2 — Integridad, accesibilidad y operación

**A08. Se acepta un equipo que no participa en el partido.** Confirmado por POST y consulta SQLite en un partido manual futuro: `SavePick` devuelve HTTP 200 y persiste un tercer equipo existente. `handlers/picks_handler.go:275-302`; `db/repository.go:1244-1258`. Evidencia: [validación](audit-evidence/pick-validation.txt). Rechazar equipos ajenos antes de guardar en todas las rutas, incluyendo carga masiva y cambios administrativos; probar identificadores inválidos y bloqueo. Esfuerzo S.

**A09. Modales sin patrón accesible completo.** Confirmado en fuente de `game_stats_modal.html:1-42`, `picks_readiness_modal.html:1-5` y `base.html:833-860`: falta semántica dialog, nombre y gestión explícita del foco. La reproducción dinámica de preparación quedó bloqueada por A01; no se afirma navegación de lector de pantalla probada. Completar foco inicial, contención y retorno al activador, Escape y nombre accesible. Esfuerzo M.

**A10. Feedback de guardado depende de toast breve.** `templates/layouts/base.html:652-669` y `static/js/app.js:4-17`: mensajes de tres segundos, truncados y sin región viva. Confirmado por fuente; no se probó anuncio con lector de pantalla. Añadir estado persistente por acción y anuncios apropiados, especialmente para fallos de red. Esfuerzo S.

**A11. Fallback offline no entrega una página utilizable.** `static/sw.js:3-6,32-39` precachea manifest e icono; no almacena las páginas y retorna `caches.match` sin garantizar Response. Confirmado por código, pendiente de prueba offline completa. Ofrecer pantalla offline explícita y recuperación; no cachear datos privados indiscriminadamente. Esfuerzo M.

**A12. CI publica imágenes sin ejecutar la suite.** `.github/workflows/docker-publish.yml` contiene build/push sin tests, vet ni análisis de dependencias. Confirmado leyendo el único workflow versionado. Añadir gates antes de publicación, pruebas de templates renderizadas y matriz de base de datos. Esfuerzo M.

**A13. Interfaz móvil densa y texto operativo pequeño.** Valoración visual, no incumplimiento WCAG por tamaño aislado. El selector de jornada de live a 360 px llega a x=364,52 y se recorta. Login: campos de 12 px y etiquetas de 11 px; picks: numerosos controles antes del primer partido, metadatos de 9–10 px y acciones fuera del ancho inicial. Evidencia y contexto en [UI/UX](audit-evidence/ui-ux.md). Priorizar selección/progreso/guardado y agrupar herramientas secundarias; comprobar legibilidad y zoom. Esfuerzo M.

**A15. Estadísticas simuladas se persisten como si fueran datos disponibles.** Confirmado por código y harness que persistió 14.068 bytes de resumen sintético al abrir un partido manual final: `handlers/live_handler.go:681-700` llama a `GenerateRealisticSummary` si ESPN no aporta estadísticas, guarda el resultado y completa probabilidades; `services/espn/fallback.go:155-177` genera datos mediante PRNG. El atajo de `live_handler.go:651-653` acepta HasStats en partidos finales y evita volver a consultar ESPN. Impacto: usuarios pueden ver estadísticas/probabilidades inventadas sin procedencia explícita; no se demostró alteración de puntos, cuyo cálculo usa marcadores. Mostrar indisponibilidad o estimaciones etiquetadas con procedencia, separar simulación de datos oficiales y permitir actualización posterior. Probar caída y recuperación de ESPN. Esfuerzo M.

**A17. CSV conserva prefijos que una hoja de cálculo puede interpretar como fórmulas.** P2. `services/auth/auth.go:90` permite usuario `=1+1`; `handlers/admin_handler.go:1412-1414` exporta texto sin neutralización. El harness confirmó el valor intacto en los datos de exportación; no se automatizó apertura en Excel/LibreOffice ni se demostró ejecución arbitraria. Evidencia: [resultado](audit-evidence/business-result.txt). Neutralizar celdas de texto con prefijos de fórmula al exportar y validar con consumidores soportados. Esfuerzo S.

### Riesgos que necesitan validación adicional

- **Borrado de pronósticos:** COALESCE en `db/repository.go:1249-1251` conserva valores anteriores al enviar null. Confirmar primero que vaciar datos sea una operación ofrecida al usuario; no es defecto independiente si la operación significa actualización parcial.
- **CSRF y abuso:** no se identificó protección Origin/CSRF o rate limiting global en el router; SameSite Lax mitiga solicitudes entre sitios. Falta contrastar proxy, límites reales, orígenes del mismo sitio y flujos de login/reset; no se declara explotación confirmada.
- **IP del cliente:** staticcheck señala middleware.RealIP obsoleto por confianza en cabeceras. Validar la cadena de proxies antes de usar IP para autorización, auditoría o límites. No se demostró bypass de seguridad.

## Verificación y cobertura

| Comprobación | Resultado y evidencia |
|---|---|
| Suite existente `go test ./...` | Pasa en copia aislada. [Salida](audit-evidence/tests.txt). |
| `go vet ./...` | Pasa, salida vacía. [Salida](audit-evidence/vet.txt). |
| `go test -race ./...` | Pasa en todos los paquetes. [Salida](audit-evidence/race.txt). |
| Staticcheck | Cuatro avisos: RealIP, tipo de constante, strings.Title y Sprintf innecesario. [Salida](audit-evidence/staticcheck.txt). |
| Govulncheck | Cero vulnerabilidades alcanzables; una en paquetes importados y tres en módulos requeridos sin llamadas detectadas. No equivale a ausencia de riesgo. [Salida](audit-evidence/vulnerabilities.txt). |
| Gitleaks, historial | 104 commits; dos señales corresponden a SESSION_SECRET de ejemplo en DEPLOYMENT.md, no filtraciones reales confirmadas. No se probaron contra servicios. [Salida redactada](audit-evidence/secrets.json). |
| PostgreSQL 17 | Conexión y DDL correctos; semillas fallan. Salud devuelve OK pese a falta de datos. |
| HTTP autenticado | Admin, perfil, leaderboard, live, teams, stats, rules y playoffs entregan HTML cerrado. Picks falla: A01. Esto no prueba todas sus interacciones. |
| Visitante | Admin, profile y picks redirigen a login. |
| Reglas/permisos | Suite existente cubre bloqueo, fair play, scoring/playoffs, beta, verificación, registro y administración. No se confunde cobertura automatizada con recorridos manuales completos. |
| UI/UX | Fuente y Firefox reales; 69 combinaciones completadas de 70 intentadas en 360, 390, 768, 1366 y 1440 px; una interrumpida por diagnóstico. Sin overflow global en las muestras, lo cual no descarta clipping. Fallos de plantilla en picks. [Barrido](audit-evidence/browser-sweep.json); roles, teclado y límites en informe UI/UX. |
| Detector Impeccable | 70 señales, no 70 defectos: URLs de templates y colores contextuales generan posibles falsos positivos. [JSON](audit-evidence/detector.json). |

Arquitectura: hay separación de servicios y handlers, pero repository (5.838 líneas), models (2.363) y layout (1.215) concentran múltiples responsabilidades. Es una oportunidad de separación por dominio después de resolver defectos; su tamaño por sí solo no acredita fallo funcional. RenderPage recompila plantillas por petición: medir latencia/carga antes de introducir cachés. Las respuestas HTML observadas llegan aproximadamente a 450 KB sin comprimir en live; revisar payload y parciales antes de atribuir lentitud sin métricas.

Fortalezas: bcrypt, HMAC con comparación constante, tokens aleatorios criptográficos, SQL parametrizado para SQLite, controles de autenticación/verificación/admin, bloqueo de juegos en servidor, pruebas de fair play, suite funcional amplia y runtime con reducción de privilegios.

## Secuencia de resolución y límites

1. Cerrar exposición de secretos (A03/A04) y verificar configuración productiva.
2. Reparar renderizado de picks y atomicidad del recálculo (A01/A16), con regresiones de documento completo y rollback.
3. Resolver sesiones/cookies (A05/A06) y validación de picks (A08); cerrar SSRF push (A14).
4. Corregir PostgreSQL, procedencia de estadísticas y gates de CI (A02/A15/A12).
5. Completar accesibilidad y feedback (A07/A09/A10), después simplificar móvil y offline (A13/A11).
6. Repetir suite y recorridos afectados; terminar con pulido visual y nueva auditoría técnica de UI.

No se realizaron pruebas destructivas, carga sostenida, restore de backups, despliegue Docker completo de la aplicación, comprobación SMTP/push real, validación de HSTS/TLS productivo, Safari/iOS físico ni certificación de lector de pantalla. No se validaron todos los estados de cada endpoint. Las recomendaciones de operación son revisión de código y configuración, no comprobación de infraestructura. La lista refleja defectos y riesgos encontrados, no garantiza ausencia de otros.
