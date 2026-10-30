# Tercera auditoría — commit 72dd947

30 de octubre de 2026. Snapshot `72dd947b0de4f130229195057137b7f591972f98`, comparado con b7070be. Servicio público, copias temporales, datos ficticios y pruebas locales. Sin correcciones de aplicación ni intervención productiva.

## Dictamen

**Hay mejoras verificadas, pero aún no procede cerrar todos los hallazgos.** PostgreSQL ya inicializa la temporada, equipos y administrador; sin embargo, varias operaciones siguen fallando por incompatibilidades booleanas. La reproducción de DNS rebinding ahora se bloquea, y el recálculo revierte correctamente y devuelve error administrativo. La nueva protección de origen y confianza en proxies requiere ajustes y pruebas de despliegue.

Fuentes de detalle: [backend](audit-third-review/backend.md) y [UI/UX](audit-third-review/ui.md). Los informes anteriores permanecen como evidencia de sus commits, no del estado actual.

## Hallazgos abiertos y riesgos nuevos

### P1 — A02: PostgreSQL sigue incompleto

Confirmado en PostgreSQL real: la siembra termina sin error y crea 32 equipos, 22 semanas y16 juegos; creación de usuario y partido también funcionan. Pero estadísticas de usuario, verificación, beta, conteo/lectura de notificaciones y lectura de leaderboard fallan por boolean=integer, parámetros enteros para bool o COALESCE boolean/integer. [CRUD](audit-third-review/pgcrud.txt), [siembra](audit-third-review/postgres.txt). No basta sustituir literales de INSERT: auditar parámetros, SELECT, filtros y agregaciones; ejecutar una matriz real de operaciones por dialecto antes de declarar compatibilidad completa.

### P2 — N01: CSRF no compara el origen completo

`services/security/csrf.go:75-92` compara hosts y elimina puertos. Una solicitud POST cuyo origen es `http://quiniela.example:9999` se acepta para `https://quiniela.example`; un origen de host distinto sí se rechaza. Esquema y puerto forman parte del origen. Una aplicación no confiable en otro puerto del mismo host puede quedar autorizada; el impacto depende del despliegue. Ausencia de Origin/Referer también pasa, pero eso por sí solo no acredita explotación en un navegador moderno. [Reproducción controlada](audit-third-review/parent-security.txt). Comparar orígenes completos y definir explícitamente excepciones para proxy/desarrollo; comprobar compatibilidad con formularios y HTMX.

### P2 — N02: proxies explícitos no restringen la confianza predeterminada

`services/security/clientip.go:37-84` siempre conserva rangos privados cuando behindProxy=true y añade los explícitos. Configurar únicamente172.18.0.2 no evita que10.5.6.7 suministre un X-Forwarded-For aceptado. [Prueba](audit-third-review/parent-security.txt). La suplantación desde Internet depende de acceso al backend y de cómo el proxy sanea las cabeceras; no se afirma bypass productivo demostrado. Hacer que la lista explícita delimite la confianza y probar la cadena efectiva. Esto importa porque los límites dependen de la IP resultante.

### P2 — A15: procedencia de estadísticas históricas

La bandera identifica resúmenes nuevos, pero un JSON sintético histórico sin bandera sigue sin mostrar etiqueta: `SIMULATED_LEGACY label=false`. [Resultado](audit-third-review/sqlite.txt). Determinar y migrar/inutilizar los registros sintéticos anteriores sin presentarlos como oficiales; comprobar también datos mezclados y no solo el resumen nuevo.

## Matriz A01–A17

Estado basado en evidencias de esta ronda; el alcance específico y los límites están en los anexos. Resuelto significa superado en los escenarios probados, no garantía exhaustiva.

| ID | Estado |
|---|---|
| A01 | Resuelto en escenarios de documento completo e históricos probados. |
| A02 | Parcial: siembra corregida; CRUD/leaderboard aún falla. |
| A03 | Sin regresión identificada por fuente: healthcheck no serializa DSN. |
| A04 | Producción activada en Docker/Compose; arranque con secreto predeterminado rechazado en proceso de prueba (config-production.txt). |
| A05 | Resuelto en reproducción de cambio/reset y cookie legacy. |
| A06 | Resuelto en TLS y proxy HTTPS probados; no se certifica proxy real. |
| A07 | Labels asociados; ver límites de autofill y legibilidad en UI/UX. |
| A08 | Repositorio y POST individual rechazan equipo ajeno; borrado/masivo requiere cobertura específica. |
| A09 | Resuelto en readiness probado: foco inicial dentro, Tab contenido y Escape retorna activador. |
| A10 | Parcial: temporización5/8s y cierre manual mejorados; falta estado persistente de la acción. |
| A11 | Resuelto para navegación: servidor detenido y URL nueva reciben pantalla offline200 desde SW; assets completos no certificados. |
| A12 | CI usa go.mod y tests/vet condicionan publicación; sin ejecución remota nueva. |
| A13 | Selector correcto en cinco anchos; densidad/legibilidad general aún requiere mejora. |
| A14 | Rebinding bloqueado con transporte nuevo; redirecciones/proxy y límites descritos en backend. |
| A15 | Parcial: históricos sin bandera todavía no identificados. |
| A16 | Resuelto en rollback, reintento y HTTP500 administrativo ante fallo. |
| A17 | Resuelto en CSV para prefijos de fórmula probados; sin apertura real en Excel. |

## Verificaciones generales y límites

Suite, vet, detector de carreras y compilación pasan en copia aislada. [Tests](audit-third-review/tests.txt), [race](audit-third-review/race.txt), [vet](audit-third-review/vet.txt), [build](audit-third-review/build.txt). Staticcheck no emite avisos en esta versión ([salida](audit-third-review/staticcheck.txt)); govulncheck y su alcance se registran en [salida](audit-third-review/vulnerabilities.txt).

Pruebas de red solo contra señuelos/control de DNS y transporte de prueba; sin contactar infraestructura privada real. Sin SMTP/push a terceros, despliegue ni carga sostenida. Casos no realizados, incluido zoom nativo si el navegador no lo ejecuta, no se presentan como verificados. No se certifica WCAG ni infraestructura real. Los harnesses adjuntos permiten reproducir en copias aisladas; no se deben ejecutar con datos o credenciales productivos.

Recursos: los anexos registran el cierre de servidores, navegador y contenedor propios. No se alteraron los informes históricos. Compilación y análisis se ejecutaron en una copia del snapshot; los harnesses adicionales solo existen allí o como evidencia textual.
