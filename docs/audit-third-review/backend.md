# Tercera revisión backend: 72dd947 frente a b7070be

Revisión de alcance acotado ejecutada sobre snapshot `git archive 72dd947`, sin modificar aplicación ni bases existentes. Datos/claves ficticios, SQLite temporal y PostgreSQL17-alpine vacío en contenedor propio. Se conservaron fuentes y resultados junto a este informe. Índice MCP fechado27-oct: cobertura consultada, archivos nuevos de security no indexados y demás fuentes cambiadas, por lo que conclusiones se verificaron leyendo fuente actual.

## Matriz de revalidación

| ID | Estado actual | Evidencia |
|---|---|---|
| A02 | **Parcial, abierto P1** | PG ya migra y siembra32 equipos,22 semanas,16 juegos; permite usuario, partido, pick y cálculo. Siguen fallando perfil/estadísticas, verificación/beta, notificaciones y lectura de posiciones. postgres.txt / pgcrud.txt. |
| A04 | Mejorado: guardas productivas y despliegue | Docker/compose activan production y config detecta /app/data; rechaza valores predeterminados exactos y no imprime contraseña. No se repitió arranque de contenedor app; revisión de fuente y contraste con evidencia del coordinador. No acredita fortaleza de cualquier valor personalizado. |
| A14 | **DNS rebinding directo corregido; redirects bloqueados por implementación** | Harness DNS original contra nuevo transporte:2 consultas, segunda127.0.0.1, local_hits=0, error SSRF. El cliente ahora prohíbe todo redirect. Proxy ambiental merece validación de despliegue; ver límites. |
| A15 | **Parcial, P2 histórico** | Nuevo resumen flag=true/etiqueta=true; JSON equivalente al legado sin marca sigue etiqueta=false. Ganó flag de winprob simulado en modal, pero no saneamiento de histórico. sqlite.txt. |
| A16 | **Corregido en escenario probado** | Fallo de inserción tabla revierte picks0/board0, retorna error; handler admin ahora HTTP500/error; reintento17/17 y repetición sin error. sqlite.txt. |
| A17 | **Corregido para prefijos probados** | Exportación real neutraliza =,+,-,@ y fórmula precedida por espacios/tab; texto normal intacto. sqlite.txt. |

## P1: A02 compatibilidad PostgreSQL aún incompleta

`db/repository.go:474` compara `p.is_correct = 1`; `:2109-2120` compara/asigna is_read con0/1; `:2318-2345` suministra enteros a parámetros BOOLEAN. `GetWeeklyLeaderboard` usa `COALESCE(wl.tiebreaker_winner_correct,0)`.

**Reproducción real (pgcrud.txt):** GetUserStats falla SQLSTATE42883 (boolean=integer); SetUserEmailVerified y SetUserBetaTester fallan al codificar1 como bool; crear notificación pasa, conteo y marcar leída fallan42883/42804; SavePick, UpdateGameScoreAndStatus y CalculateWeekScores pasan; GetWeeklyLeaderboard falla42804 por COALESCE boolean/integer. La corrección de semillas no basta para operar el servicio con PG.

Recomendación: revisar todo SQL/argumento booleano por dialecto y añadir integración PG sobre perfil, roles/verificación, preferencias, notificaciones y posiciones, además de arranque/CRUD. No declarar compatibilidad completa hasta que esos flujos pasen.

## P2: A15 datos históricos sin procedencia

`handlers/live_handler.go:653` sigue aceptando HasStats=true sin IsSimulatedFallback como reporte oficial. Datos persistidos antes de introducir el flag se leen con false y el modal omite aviso. La prueba crea resumen sintético, elimina su marca como en formato legado y abre modal:200/label=false. No se comprobó una migración que identifique o invalide dichos resúmenes. El nuevo HasSimulatedWinProb mejora la mezcla parcial del modal, pero no identifica datos históricos. No se demostró alteración de puntos.

## Nuevas protecciones de seguridad: límites comprobados

- **CSRF, P2 contextual:** `services/security/csrf.go:76-94` compara host sin esquema y descarta puertos. Acepta un Origin http://mismo-host:9999 contra destinoHTTPS. La prueba del coordinador está en parent-security.txt. Es una política más amplia que mismo origen: riesgo si hay contenido no confiable en otro puerto/esquema del mismo host. Ausencia de Origin/Referer también permite solicitud, pero eso por sí solo NO demuestra explotación en navegador moderno. Comparar origen completo y definir excepciones de proxy explícitas.
- **Proxy/IP, P2 de configuración:** `services/security/clientip.go:40-60` confía en toda RFC1918/ULA con behindProxy=true, incluso al definir TRUSTED_PROXIES estrecho. El coordinador confirmó que un remoto10.5.6.7 no enumerado puede imponer XFF8.8.8.8 aunque se configure solo172.18.0.2. Si otros clientes/servicios alcanzan directamente el contenedor, permite rotar IP aparente y eludir límites. Si solo un proxy de confianza puede alcanzarlo y normaliza cabeceras, el riesgo se reduce. Lista explícita debería reemplazar defaults amplios.
- **Rate limiting:** revisado por fuente: token bucket por IP con429/Retry-After, limpieza periódica y limitador separado de auth; su eficacia depende de la resolución IP anterior. No se ejecutó prueba masiva ni se certifica memoria bajo IPs distribuidas. El estado es por proceso, no compartido entre réplicas.

## Transporte push y límites de evidencia

El nuevo DialContext (`services/notifications/webpush.go:123-145`) revalida IP y conecta a la IP elegida, evitando la resolución secundaria insegura que se reprodujo enb7070be. **Prueba dinámica real de nuevo transporte:** dns.txt demuestra rechazo antes del POST, switch de tests=false y socket señuelo0 visitas. No se modificó transporte de aplicación en esa prueba.

CheckRedirect (`:163-167`) devuelve ErrUseLastResponse para todos los saltos. No se atribuye al nuevo transporte la prueba antigua basada en http.DefaultTransport: desde este commit ese reemplazo ya no controla el cliente y el harness antiguo no es válido para demostrar el recorrido del redirect. El transporte usa ProxyFromEnvironment (`:121`): con proxy se valida la IP del proxy y la resolución final puede ocurrir fuera de este proceso. No se ejecutó bypass de proxy; exigir proxy de confianza que imponga la misma política o desactivar proxies para push. No se garantiza protección de toda configuración ambiental por esta revisión.

## Recursos y reproducción

Los archivos *-repro.go.txt contienen programas main que se ejecutan individualmente dentro de una copia aislada del módulo. postgres/pgcrud requieren PostgreSQL vacío ficticio en127.0.0.1:18434; sqlite/dns crean y limpian sus propios recursos. Se detuvo y eliminó `nfl-audit-third-backend-pg`; no se dejó servidor de aplicación. No se tocaron archivos de aplicación ni reportes históricos de auditoría. No se repitieron suites/vet/race que ejecuta el coordinador.
