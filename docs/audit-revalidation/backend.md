# Revalidación técnica

Evaluación independiente iniciada por el agente técnico y consolidada por el coordinador tras interrupción de capacidad. Evidencias recuperadas de los harnesses terminados, sin modificar aplicación. Los fuentes *-repro.go.txt se ejecutan como main.go en carpeta temporal dentro de una copia del módulo; postgres usa instancia de prueba. No usar credenciales reales.

## Resultados

- A02 parcial P1: PostgreSQL17 sigue fallando en notify_email/is_active boolean frente integer; teams32, seasons/weeks/games0. postgres-result.txt.
- A03 resuelto por inspección: main.go ya no serializa db_path. No se ejecutó healthcheck de aplicación completa PostgreSQL en esta ronda.
- A05 resuelto en harness: sesión rechazada tras cambio/reset; formato legacy también revocado. sqlite-result.txt.
- A06 resuelto en escenarios TLS y X-Forwarded-Proto=https; HTTP conserva Secure=false. No certifica configuración proxy real. sqlite-result.txt.
- A08 resuelto en repo y POST individual: equipo tercero rechazado con400. Rutas masivas no probadas dinámicamente en esta ronda.
- A14 parcial P1: loopback directo rechazado pero redirect308 y cambio de resolución DNS permiten POST a loopback con switch de pruebas false. ssrf-result.txt y dns-result.txt. Conexiones controladas, sin sondear redes reales.
- A15 parcial P2: resumen nuevo tiene bandera y etiqueta; resumen histórico sin bandera no la muestra. live_handler.go:696-704 mezcla probabilidades sintéticas sin marcar el resumen oficial. sqlite-result.txt.
- A16 parcial P2: transacción revierte pick y leaderboard ante fallo y permite retry17/17; SaveGameScore en admin_handler.go:368-377 registra fallo pero devuelve200 con toast success. sqlite-result.txt.
- A17 resuelto en exportación descargada: prefijos =,+,-,@ y tab/espacio neutralizados. No apertura automática en Excel. sqlite-result.txt.

Las verificaciones generales y configuración están en el informe principal. Ninguna conclusión declara seguridad exhaustiva de producción.
