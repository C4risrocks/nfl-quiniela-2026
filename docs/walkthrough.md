# Walkthrough de correcciones

Este documento describe cambios implementados y sus comprobaciones. No certifica ausencia de defectos ni compatibilidad de cualquier despliegue. Los informes de auditoría anteriores conservan el estado de sus respectivos commits.

## Persistencia

Se corrigen consultas, parámetros y valores nulos booleanos para SQLite y PostgreSQL. La ventana temporal de jornada se calcula sin depender de funciones exclusivas de SQLite. La matriz de integración incluye siembra, usuarios/verificación/beta, notificaciones, pronósticos, recálculo y posiciones; véase [validación](validacion-correcciones.md).

## Origen y proxies

[CSRF](../services/security/csrf.go) compara esquema, hostname y puerto efectivo con APP_BASE_URL. Usa Origin y, si falta, Referer; sin ambos exige Sec-Fetch-Site: same-origin. Solicitudes rechazadas reciben403 y feedback HTMX.

[Resolución de IP](../services/security/clientip.go) solo acepta cabeceras desde proxies explícitos con BEHIND_PROXY=true. No añade automáticamente loopback ni rangos privados. La misma confianza determina el esquema reenviado utilizado para cookies. La configuración inválida o incompleta impide arrancar. Consulte [despliegue](../DEPLOYMENT.md).

## Push

[El transporte](../services/notifications/webpush.go) valida destinos públicos HTTPS y conecta a una IP validada; no sigue redirecciones ni usa proxies ambientales. Las dependencias de red son por instancia. Los tests de redirección comprueban que la respuesta inicial realmente se recibió y que el destino prohibido recibió cero solicitudes; cualquier error inesperado falla. Rebinding y entrega/pruning tienen pruebas separadas. No se utiliza un interruptor global para relajar la seguridad.

## Estadísticas e interfaz

Los resúmenes distinguen fuente oficial, simulada y no verificada. Los datos históricos sin procedencia se conservan con advertencia y pueden actualizarse con información oficial; no se borran masivamente. Las superficies en vivo y los modales comparten esta distinción.

El enfoque inicial se aplica al abrir un diálogo; los refrescos conservan la interacción. Los resultados de guardado se muestran junto a la acción, además del toast. Los errores específicos de seguridad no deben sustituirse por mensajes genéricos.

## Evidencia y límites

Los comandos, resultados, alcance y pendientes están en [validacion-correcciones.md](validacion-correcciones.md), con salidas en [fix-validation](fix-validation/). Que una suite pase acredita sus escenarios, no todos los comportamientos posibles. No se verificó infraestructura productiva, TLS real, lector de pantalla ni entrega a dispositivos push externos.
