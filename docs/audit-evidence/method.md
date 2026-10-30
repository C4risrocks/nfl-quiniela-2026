# Método reproducible y entorno

Snapshot a6dc4c98e9088640b6ee423effa9360c137e3de5. macOS arm64; Go go1.27.3; Docker Engine 29.6.2. Pruebas sobre `git archive HEAD` extraído a carpeta temporal. El repositorio original solo recibe documentación/evidencias.

Comandos de verificación en esa copia:

```sh
go test ./...
go vet ./...
go test -race ./...
staticcheck ./...
govulncheck ./...
```

Staticcheck y govulncheck se instalaron en GOBIN temporal, sin cambiar go.mod/go.sum de la aplicación. Gitleaks se ejecutó con `gitleaks git --redact --no-banner --report-format json`; las dos alertas se inspeccionaron y corresponden a placeholders documentados en versiones históricas.

Arranque SQLite con PORT=18089, DB_PATH y UPLOAD_DIR temporales, secreto/contraseña ficticios, ENABLE_BACKGROUND_SYNC=false, ENABLE_REMINDERS=false y SMTP vacío. El arranque aún hace consultas iniciales de solo lectura a ESPN. No se asume determinismo de datos externos: el caso de equipo inválido usó partido manual futuro sin ID ESPN y el de sesión una cuenta ficticia creada para ello.

PostgreSQL: contenedor `postgres:17-alpine`, puerto ligado a 127.0.0.1:15439, base vacía `audit`. Segunda instancia con DB_TYPE=postgres, DATABASE_URL ficticia y PORT=18090. Las credenciales temporales se redactaron en la evidencia. El fallo se reprodujo antes de cualquier cambio de esquema externo. No se aplicaron correcciones de SQL.

Interfaz: Playwright CLI con Firefox; las evaluaciones A (visual/UX) y B (detector/fuente técnica) se hicieron en contextos de agentes independientes antes de consolidar sus resultados. El detector escaneó templates; no equivale a auditoría WCAG renderizada. El índice codebase-memory se usó con comprobación de cobertura y lectura directa en archivos parciales/cambiados.

No se publicaron imágenes Docker, issues, PRs ni mensajes en servicios externos. La única petición de prueba SSRF fue al servidor HTTP señuelo de loopback, según harness y log asociados.

Cierre: sesiones de Firefox cerradas, servidores 18089/18090 detenidos con SIGTERM y contenedor PostgreSQL detenido/eliminado. Comprobación final: git diff sin cambios de aplicación; solo docs/ añadido.
