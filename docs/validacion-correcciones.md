# Validación de correcciones

Fecha: 30 de octubre de 2026. Base `125c4ed`; resultados sobre el árbol de trabajo corregido, todavía sin commit nuevo. Informes anteriores preservados. Esta validación acredita los escenarios enumerados y no certifica ausencia de otros defectos.

## Cambios e interfaces

- PostgreSQL/SQLite: parámetros y expresiones booleanas nativas, lectura de flags como booleanos, rebind también para consultas preparadas de DB/Tx y ventana de jornada calculada en Go. Se conservan tablas, prioridades y reglas de puntuación.
- CSRF: origen público exacto normalizado, incluyendo esquema y puerto. Origin precede a Referer; sin ambos se exige Sec-Fetch-Site: same-origin. No hay excepciones automáticas para otros puertos o localhost/127.0.0.1.
- Proxy: BEHIND_PROXY=false ignora cabeceras. Con true, TRUSTED_PROXIES es explícito, exhaustivo y obligatorio. Configuración inválida impide arrancar; el esquema reenviado para cookies usa la misma confianza.
- Push: constructor público conservado, dependencias DNS/dial internas por instancia, sin interruptor global de seguridad, sin proxies ambientales ni seguimiento de redirects.
- Resumen de estadísticas: campo JSON `source` con espn/simulated/unknown. Compatibilidad con la bandera antigua de simulación. Histórico sin procedencia conservado y advertido; revalidación oficial mediante caché existente, sin borrado o backfill masivo. No se mezclan líderes/sede/probabilidades inventadas con resumen oficial.
- UI: foco al abrir y preservado en refrescos; estado persistente de guardado/error; errores específicos de seguridad conservados.

## Resultado

Pasan suite global, vet, race, compilación y staticcheck. PostgreSQL17.11 real y SQLite pasan la matriz de integración con race. Govulncheck informa cero vulnerabilidades alcanzables; conserva avisos de una vulnerabilidad en paquetes importados y tres en módulos requeridos sin llamadas detectadas.

La [verificación UI](fix-validation/ui.md) confirma guardado real con progreso50%→100%, errores sin pérdida de selección, procedencia visible y foco/valor/selección conservados en refrescos. Tras los últimos ajustes de textos y comentarios se repitió [handlers](fix-validation/handlers-final.txt); los checks globales preceden únicamente esos ajustes finales. El detector visual conserva70señales del sistema existente, que no se declaran solucionadas por esta tarea.

## Comandos y evidencias

Las pruebas globales finales se ejecutaron en una copia separada del árbol modificado, excluyendo credenciales, bases y entornos virtuales. Una ejecución inicial colisionó con otro test que usaba test_handlers.db fijo; el helper se corrigió con t.TempDir y la comprobación final se repitió aislada. Las salidas enlazadas corresponden a la ejecución final.

| Comprobación | Evidencia |
|---|---|
| go test ./... | [tests.txt](fix-validation/tests.txt) |
| go vet ./... | [vet.txt](fix-validation/vet.txt) |
| go test -race ./... | [race.txt](fix-validation/race.txt) |
| go build ./... | [build.txt](fix-validation/build.txt) |
| staticcheck ./... | [staticcheck.txt](fix-validation/staticcheck.txt) |
| govulncheck ./... | [vulnerabilities.txt](fix-validation/vulnerabilities.txt) |
| Configuración, orígenes, proxies y cookies | [security.txt](fix-validation/security.txt) |
| Integración real SQLite/PostgreSQL17.11 con race y límites de jornada | [postgres.txt](fix-validation/postgres.txt) |

La integración usa esquemas PostgreSQL únicos por prueba y los elimina al finalizar. Se exige TEST_POSTGRES_DSN con REQUIRE_POSTGRES_TESTS=1 para impedir omitir PostgreSQL en CI. Ejecutar `go test -v -race -tags=integration ./db` con ese entorno; la suite normal no requiere Docker.

Cobertura de integración: semillas, flags, usuarios individual/masivo/administrativo, verificación/beta, preferencias, notificaciones y ownership, picks, cálculo y tabla semanal/temporada, perfiles/comunidad, consultas preparadas de probabilidades y eliminación. Once casos de jornada cubren prioridades, límites inclusivos, ventanas superpuestas y cambio de martes CDMX.

Pruebas push: respuestas iniciales302/307/308 efectivamente recibidas con cero visitas al destino interno; rebinding y conjuntos mixtos de IP bloqueados antes del dial; IPv4/IPv6; claves válidas para entrega/pruning. Errores de red o cifrado inesperados fallan la prueba. Los destinos son fixtures locales controlados, sin contactar proveedores externos.

CI ejecuta vet, race e integración con servicio PostgreSQL17 en PRs hacia main y pushes a main. Publicación de imagen depende de los checks y solo se ejecuta en push a main; no se lanzó un workflow remoto durante esta tarea.

## Estado de hallazgos

| Hallazgo | Resultado de esta implementación |
|---|---|
| A01 | Preservado: regresiones de renderizado en suite, sin documentos parcialmente escritos. |
| A02 | Corregido en la matriz SQLite/PostgreSQL enumerada; no se extrapola compatibilidad universal. |
| A03 | Corrección previa conservada; sin DSN en healthcheck por fuente. |
| A04 | Configuración validada y despliegue documentado; credenciales externas requeridas en Compose. |
| A05 | Revocación de sesiones cubierta por suite existente. |
| A06 | Cookies usan TLS o esquema de proxy explícitamente autorizado. |
| A07 | Etiquetas de formularios conservadas; no certificación de lector de pantalla. |
| A08 | Validación de equipo conservada; pruebas de persistencia y borrado existentes incluidas. |
| A09 | Enfoque inicial separado de refrescos; comprobación de navegador en evidencia UI. |
| A10 | Estado persistente por acción y preservación de mensajes específicos; toast suplementario. |
| A11 | Fallback anterior conservado; no instalación offline fría certificada. |
| A12 | Gates de integración incorporados; ejecución remota pendiente del push/PR del usuario. |
| A13 | Sin rediseño general: los pendientes de densidad/legibilidad no se declaran cerrados. |
| A14 | Pruebas SSRF sin falsos pases y transporte con política estricta. |
| A15 | Procedencia explícita o desconocida visible sin borrar histórico. |
| A16 | Atomicidad y respuesta de error anteriores conservadas por suite. |
| A17 | Neutralización CSV anterior conservada; sin apertura real en Excel. |
| N01 | Comparación de origen completo y política explícita ante metadatos ausentes. |
| N02 | Lista de proxies exhaustiva, sin rangos privados añadidos. |

## Despliegue y límites

Antes de desplegar, definir APP_BASE_URL exacto y seleccionar acceso directo o proxy. Si BEHIND_PROXY=true, identificar las IP/CIDR reales del proxy; no copiar redes amplias por conveniencia. Push requiere salida HTTPS directa. Ver [DEPLOYMENT.md](../DEPLOYMENT.md).

Sin despliegue, correos reales, push a dispositivos externos, carga sostenida, lector de pantalla, Safari físico ni zoom nativo certificado. Los cambios de estadísticas añaden un campo JSON; no hay migración destructiva ni modificación del esquema para procedencia. Los históricos oficiales sin marca se muestran conservadoramente como no verificados hasta una consulta oficial exitosa.

Los servidores y navegadores propios de validación están cerrados; el contenedor PostgreSQL temporal fue eliminado. Los informes históricos no se modificaron.
