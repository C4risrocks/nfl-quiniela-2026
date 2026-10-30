# Walkthrough: Resolución de Riesgos Residuales y Segunda Revalidación de Auditoría

Este documento certifica la implementación, resolución y verificación rigurosa de los riesgos residuales y los puntos identificados en la revalidación de auditoría ([`docs/auditoria-revalidacion.md`](file:///Users/christiandejmorenomarin/Documents/nfl-quiniela-2026/docs/auditoria-revalidacion.md)) en la rama **`dev`**.

---

## 🛡️ Mejoras y Blindajes Implementados

### 1. Resolución Confiable de IP del Cliente (Sustitución de `RealIP` vulnerable)
- **Problema previo**: `middleware.RealIP` de chi estaba deprecado (GHSA-3fxj-6jh8-hvhx / SA1019) y mutaba ciegamente `r.RemoteAddr` permitiendo IP spoofing si el cliente inyectaba cabeceras `X-Forwarded-For`.
- **Solución implementada**:
  - Se creó [`services/security/clientip.go`](file:///Users/christiandejmorenomarin/Documents/nfl-quiniela-2026/services/security/clientip.go) con [`ClientIPResolver`](file:///Users/christiandejmorenomarin/Documents/nfl-quiniela-2026/services/security/clientip.go#L30-L120).
  - Solo se confía en cabeceras de proxy (`X-Forwarded-For`, `X-Real-IP`) si la conexión física directa proviene de un proxy confiable (`TRUSTED_PROXIES`, loopback `127.0.0.1`, o subredes privadas RFC 1918 cuando `BEHIND_PROXY=true`).
  - La inspección de `X-Forwarded-For` se realiza de derecha a izquierda (estándar RFC 7239) para localizar la primera IP no confiable.
  - Se sustituyó `middleware.RealIP` en [`main.go`](file:///Users/christiandejmorenomarin/Documents/nfl-quiniela-2026/main.go#L180-L195).

### 2. Protección CSRF / Same-Origin en Solicitudes Mutantes
- **Problema previo**: Ausencia de verificación de origen en solicitudes POST/PUT/DELETE.
- **Solución implementada**:
  - Se creó [`services/security/csrf.go`](file:///Users/christiandejmorenomarin/Documents/nfl-quiniela-2026/services/security/csrf.go) con [`CSRFProtectionMiddleware`](file:///Users/christiandejmorenomarin/Documents/nfl-quiniela-2026/services/security/csrf.go#L10-L75).
  - Intercepta métodos mutantes (`POST`, `PUT`, `DELETE`, `PATCH`).
  - Bloquea inmediatamente peticiones con metadatos de navegador cruzados (`Sec-Fetch-Site: cross-site`) y orígenes que no coincidan con el host (`r.Host`) o `APP_BASE_URL`.
  - Emite respuesta HTTP 403 Forbidden y toast de error informativo.

### 3. Rate Limiting Multinivel en Memoria
- **Solución implementada**:
  - Se creó [`services/security/ratelimit.go`](file:///Users/christiandejmorenomarin/Documents/nfl-quiniela-2026/services/security/ratelimit.go) con [`IPRateLimiter`](file:///Users/christiandejmorenomarin/Documents/nfl-quiniela-2026/services/security/ratelimit.go#L18-L100) (algoritmo Token Bucket thread-safe por IP con limpieza periódica de memoria).
  - **Límite Global** ([`GlobalRateLimitMiddleware`](file:///Users/christiandejmorenomarin/Documents/nfl-quiniela-2026/services/security/ratelimit.go#L105-L145)): 120 peticiones/minuto por IP para tráfico dinámico, con bypass automático para `/static/*`, `/favicon.ico`, `/robots.txt`, `/sw.js` y `/events` (SSE streaming).
  - **Límite de Autenticación** ([`AuthRateLimitMiddleware`](file:///Users/christiandejmorenomarin/Documents/nfl-quiniela-2026/services/security/ratelimit.go#L150-L180)): 10 peticiones/minuto por IP para `/login`, `/register`, `/forgot-password`, `/reset-password` y `/verify-email/resend`. Emite cabecera HTTP estándar `Retry-After`.

### 4. Consistencia en Guardado y Borrado de Pronósticos (`DATA-02`)
- **Problema previo**: `COALESCE` en `db/repository.go` impedía vaciar marcadores de desempate si el usuario los borraba en un guardado posterior.
- **Solución implementada**:
  - Se agregaron [`SavePickFull`](file:///Users/christiandejmorenomarin/Documents/nfl-quiniela-2026/db/repository.go#L1300-L1330) y [`DeletePick`](file:///Users/christiandejmorenomarin/Documents/nfl-quiniela-2026/db/repository.go#L1332-L1340) en `Repository`.
  - `SaveAll` y `AdminSaveUserPicks` ahora usan `SavePickFull`, permitiendo persistir marcadores `NULL` explícitos cuando el usuario o admin los borra, y eliminando el pick con `DeletePick` si se deselecciona por completo.

### 5. Compatibilidad Estricta PostgreSQL 17 (Tipos Booleanos SQLSTATE 42804)
- **Problema previo**: `SeedDatabase` y ciertas consultas insertaban `1` y `0` literales en columnas `BOOLEAN` (`is_active`, `notify_email`, `email_verified`, `is_read`), lo cual causaba error de tipo en PostgreSQL.
- **Solución implementada**:
  - Se reemplazaron todos los literales `1` y `0` en sentencias SQL por constantes booleanas nativas estándar `TRUE` y `FALSE` en [`db/repository.go`](file:///Users/christiandejmorenomarin/Documents/nfl-quiniela-2026/db/repository.go), [`db/db.go`](file:///Users/christiandejmorenomarin/Documents/nfl-quiniela-2026/db/db.go) y esquemas, compatibles al 100% tanto con SQLite como con PostgreSQL 17.

### 6. SSRF WebPush: Bloqueo de Redirecciones y DNS Rebinding
- **Problema previo**: Posibilidad de que un endpoint público redirigiera mediante 301/302 a direcciones loopback o cambiara de IP por rebinding.
- **Solución implementada**:
  - En [`services/notifications/webpush.go`](file:///Users/christiandejmorenomarin/Documents/nfl-quiniela-2026/services/notifications/webpush.go#L120-L160), se configuró `CheckRedirect` devolviendo `http.ErrUseLastResponse` (prohibición estricta de seguir redirecciones).
  - Se añadió `DialContext` en el transporte HTTP validando la IP de destino en el momento exacto previo a la conexión TCP.

### 7. Configuración Segura de Producción y Despliegue
- En [`config/config.go`](file:///Users/christiandejmorenomarin/Documents/nfl-quiniela-2026/config/config.go#L70-L85), se detecta automáticamente el entorno de producción si existe el directorio persistente `/app/data` de contenedor.
- Se fijó `ENV APP_ENV=production` en [`Dockerfile`](file:///Users/christiandejmorenomarin/Documents/nfl-quiniela-2026/Dockerfile#L47) y `APP_ENV=${APP_ENV:-production}` en [`docker-compose.yml`](file:///Users/christiandejmorenomarin/Documents/nfl-quiniela-2026/docker-compose.yml#L11).

### 8. Accesibilidad de Toasts y Modales (A09 / A10)
- En [`templates/layouts/base.html`](file:///Users/christiandejmorenomarin/Documents/nfl-quiniela-2026/templates/layouts/base.html#L845-L870), la trampa de foco (`Tab`) ahora detecta si el foco está fuera del diálogo y lo fuerza inmediatamente adentro.
- `htmx:afterSwap` enfoca automáticamente el primer elemento interactivo del diálogo al abrir.
- El toast notification cuenta con botón de cierre manual explícito (`xmark`), mayor tiempo de visualización (8s en errores) y soporte para advertencias.

### 9. CI y Linters
- En [`.github/workflows/docker-publish.yml`](file:///Users/christiandejmorenomarin/Documents/nfl-quiniela-2026/.github/workflows/docker-publish.yml#L21), se vinculó `go-version-file: 'go.mod'` para sincronizar exactamente la versión declarada.
- Se resolvieron los 3 avisos de `staticcheck`:
  - `services/auth/auth.go`: `SessionCookieName string = "quiniela_session"` (SA9004).
  - `services/espn/client.go`: `cases.Title(language.Und).String(c.Name)` (SA1019).
  - `services/forecasting/advisor.go`: reemplazo de `fmt.Sprintf` innecesario (S1039).

---

## 🧪 Pruebas Ejecutadas

1. **Suite de Seguridad (`services/security`)**:
   ```bash
   go test -v -race ./services/security/...
   ```
   *Resultados*:
   - `PASS: TestClientIPResolver_PreventsSpoofing`: Bloqueo de cabeceras `X-Forwarded-For` no confiables.
   - `PASS: TestCSRFProtection_BlocksCrossSite`: Rechazo de POSTs cross-site y `Sec-Fetch-Site: cross-site` con HTTP 403.
   - `PASS: TestIPRateLimiter_ThrottlesBursts`: Regulación estricta por token bucket e IP independiente.
   - `PASS: TestAuthRateLimitMiddleware_BlocksAfterLimit`: HTTP 429 tras superar ráfaga con cabecera `Retry-After`.

2. **Borrado y Persistencia (`DATA-02`)**:
   ```bash
   go test -v ./handlers -run TestPickClearingAndDeletion_DATA02
   ```
   *Resultados*: Verificación de que `SavePickFull` permite vaciar marcadores de desempate sin que `COALESCE` los restaure, y `DeletePick` elimina el registro.

3. **SSRF Anti-Redirect**:
   ```bash
   go test -v ./services/notifications/... -run TestPushService_AntiRedirectAndAntiRebindingSSRF
   ```
   *Resultados*: Verificación de que peticiones WebPush no siguen redirecciones a servidores internos o loopback.

4. **Suite Completa con Detector de Carreras**:
   ```bash
   go vet ./...
   go test -race ./...
   ```
   *Resultados*: **Pasa al 100% en todos los paquetes (0 errores, 0 condiciones de carrera)**.
