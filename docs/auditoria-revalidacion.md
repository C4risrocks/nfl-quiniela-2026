# Segunda auditoría — revalidación de A01–A17

Fecha: 30 de octubre de 2026. Commit: b7070bea6321c3288db0f4310b478dddc90e46b2, comparado con a6dc4c9. Entorno local aislado, servicio público. Aplicación sin cambios; informe anterior conservado.

## Dictamen

**Las correcciones mejoran el proyecto, pero no resuelven íntegramente los 17 hallazgos.** Persisten dos riesgos altos reproducidos: PostgreSQL todavía falla por incompatibilidades booleanas y push permite alcanzar loopback mediante redirección. La configuración segura de producción sigue siendo optativa y no está activada en los archivos de despliegue.

El detalle técnico y sus reproducciones está en [backend](audit-revalidation/backend.md); la evidencia visual y funcional está en [UI/UX](audit-revalidation/ui.md). Las conclusiones siguientes distinguen arreglos verificados de soluciones parciales y límites de cobertura.

## Prioridades abiertas

- **P1 A02 — PostgreSQL:** los placeholders se convierten, pero SeedDatabase aún usa enteros donde PostgreSQL exige booleanos. Se reproducen errores SQLSTATE 42804 en notify_email e is_active; quedan 32 equipos pero no temporada/semanas/juegos y el administrador no se crea correctamente. Revisar todos los literales y comparaciones booleanas, no solo parámetros; añadir integración real por dialecto.
- **P1 A14 — SSRF por redirección:** con la excepción de pruebas desactivada, el endpoint inicial validado puede redirigir a HTTP loopback; el servidor de prueba recibió POST. El transporte simula la respuesta inicial pública y las conexiones efectivas son locales. No se sondeó infraestructura real. También se reprodujo DNS rebinding con respuestas controladas pública/loopback y protección activa. Validar cada salto y la IP efectiva al conectar, aplicar política de redirecciones y protección contra cambios DNS.
- **P1 A04 — configuración de producción incompleta:** config/config.go:68-76 rechaza valores predeterminados solo con APP_ENV=production/prod. Dockerfile, compose y ejemplo no lo activan. Una prueba de LoadConfig sin APP_ENV/ENV confirmó aceptación del secreto y contraseña predeterminados. La eliminación de contraseña de logs sí está implementada. Establecer valores seguros por defecto de despliegue y comprobar arranque desde la configuración publicada. [Prueba](audit-revalidation/environment.txt).
- **P2 A09 — foco inicial de modal:** la semántica y cierre mejoran, pero abrir preparación con Enter deja el foco en el activador; el siguiente Tab pasa a un control detrás del modal. La trampa solo funciona al llegar a sus extremos. Enfocar un elemento del diálogo al abrir y excluir elementos ocultos; probar cambios HTMX y retorno al activador.

- **P2 A15:** estadísticas históricas sin bandera y probabilidades simuladas mezcladas con resumen oficial aún no quedan identificadas.
- **P2 A16:** rollback ya protege los puntos; la acción administrativa todavía confirma éxito tras fallo del cálculo.

## Matriz de revalidación

La tabla consolida los resultados de las evaluaciones independientes y sus evidencias recuperadas. No se cierra un hallazgo únicamente por existir un cambio de código o un test nuevo.

| ID | Estado y resultado |
|---|---|
| A01 | Resuelto en escenarios probados: históricos con cero/nulos/no-cero; documento completo y preparación abre. |
| A02 | Parcial: rebind implementado; PostgreSQL falla todavía por booleanos. |
| A03 | Resuelto por fuente: se elimina db_path del healthcheck público; endpoint completo no repetido dinámicamente. |
| A04 | Parcial: contraseña fuera del log, protección de configuración no activa por defecto. |
| A05 | Resuelto en harness: cambio/reset revocan cookie; legacy también rechazada tras cambio. |
| A06 | Resuelto en TLS y proxy HTTPS probados; infraestructura productiva no inspeccionada. |
| A07 | Resuelto en campos probados: etiquetas asociadas y autocomplete. |
| A08 | Resuelto para repositorio y POST individual; rutas masivas sin prueba dinámica nueva. |
| A09 | Parcial: semántica/Escape mejoran, foco inicial aún permite salir del diálogo. |
| A10 | Parcial: región de estado y texto completo añadidos; confirmación sigue desapareciendo tras cuatro segundos. |
| A11 | Resuelto para navegación sin servidor: pantalla offline y reintento; assets completos no certificados. |
| A12 | Resuelto para tests/vet antes de publicar: build-and-push depende del job test. No hay ejecución remota de CI verificada. |
| A13 | Parcial: selector live a 360 px ya cabe (borde derecho 327 px); densidad general requiere evaluación adicional. |
| A14 | Parcial: bloqueo directo implementado; bypass por redirección reproducido. |
| A15 | Parcial: nuevos resúmenes etiquetados; históricos y mezcla de probabilidades siguen sin procedencia. |
| A16 | Parcial: rollback y retry correctos, pero admin aún responde éxito cuando falla recálculo. |
| A17 | Resuelto en CSV descargado para prefijos probados; sin apertura en Excel. |

## Comprobaciones generales

- `go test ./...`, `go vet ./...` y `go test -race ./...`: pasan en copia temporal. Salidas en [tests](audit-revalidation/tests.txt), [vet](audit-revalidation/vet.txt) y [race](audit-revalidation/race.txt).
- Staticcheck mantiene cuatro avisos, incluido middleware.RealIP obsoleto. [Salida](audit-revalidation/staticcheck.txt).
- Govulncheck no detecta vulnerabilidades alcanzables; informa dependencias con vulnerabilidades sin llamadas detectadas. [Salida](audit-revalidation/vulnerabilities.txt).
- El workflow selecciona Go 1.24 mientras go.mod declara 1.25: revisar alineación y descarga automática de toolchain; no se presenta como fallo de CI demostrado.

## Alcance y límites

Pruebas sobre copias temporales, cuentas ficticias y servicios locales. Sin correcciones de aplicación, envíos reales, publicación, intervención en producción ni pruebas de carga sostenida. Las pruebas de red reproducen comportamiento del cliente HTTP, no acreditan acceso a infraestructura productiva. Las pruebas UI se describen con su navegador y método reales, sin atribuir zoom nativo o dispositivo físico cuando no se pudieron validar.

Orden recomendado: cerrar A14, completar PostgreSQL y configuración de producción; después completar accesibilidad, feedback y offline. Repetir las reproducciones afectadas tras cada arreglo.

Las dos revisiones delegadas se interrumpieron por límites de capacidad antes de redactar sus informes. El coordinador recuperó logs, JSON y harnesses finalizados, verificó su correspondencia y consolidó los documentos adjuntos. Las pruebas pendientes están señaladas; no se presentan como realizadas.
