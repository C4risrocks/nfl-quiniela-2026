# Validación de procedencia, foco y guardado

Fecha2026-10-30. Se implementaron los cambios asignados sin rediseñar la identidad existente. Verificación con datos ficticios en copia independiente `/tmp/nfl-ui-implementation.w1JdU0`, FirefoxPlaywright a390×844 y servidor18112. APP_BASE_URL coincide con el puerto y BEHIND_PROXY=false. No se usaron datos privados ni se enviaron notificaciones.

## Comportamiento final

- **Procedencia explícita:** `GameDetailedSummary.Source` distingue espn/simulated/unknown. La deserialización de registros antiguos conserva sus datos y marca unknown; el antiguo flag de simulación sigue identificando simulated. Solo la respuesta real del clienteESPN recibe espn. Se eliminaron las mezclas de líderes, jugadores, sede y probabilidad fabricados dentro de resúmenes oficiales o históricos. Un resumen oficial parcial permanece parcial; no se reemplaza con una simulación para rellenar huecos. Una simulación completa sigue disponible cuando no existe resumen alguno, con advertencia visible. Las dos superficies —GameCenter y modal— muestran procedencia, y los títulos/footer dejaron de atribuir datos desconocidos a ESPN/FPI.
- **Foco:** la apertura HTMX enfoca inicialmente el primer control. En refrescos, conserva el control activo, valor editable y selección; si el foco estaba fuera, no lo mueve. Escape mantiene retorno al activador original, que ya no se sustituye por el elemento del polling.
- **Guardado:** estado persistente para cambios pendientes, envío, éxito y error. El formulario se envía por fetch y conserva selecciones ante fallos de red,403 y429; los botones vuelven a habilitarse. Éxito confirmado interpreta HX-Redirect y navega a la URLsaved para refrescar contador/readiness y conservar la confirmación renderizada. Si hay ediciones durante el envío, permanece el borrador y distingue la versión guardada de los cambios posteriores. El script compartido se incluye ahora en el layout. Un mensaje específico403/429 nunca se sustituye por un toast genérico.

## Pruebas

Pruebas permanentes añadidas:

- `db/summary_provenance_test.go`: clasificación de datos históricos, simulaciones y procedencia explícita; preservación exacta del JSONoriginal.
- `services/espn/summary_provenance_test.go`: respuestaESPN parcial mantiene ausentes jugadores/sede/probabilidad y recibe espn; fallback completo recibe simulated.
- `handlers/live_provenance_test.go`: resumen oficial parcial y legado desconocido se conservan sin completar campos inventados; badges correctos y sin atribución falsa a ESPN/FPI.

Ejecutadas en copia aislada: `go test ./db ./services/espn ./handlers -run 'Test(SummaryProvenance|FetchedSummaryHasOfficial|FallbackSummaryIsExplicitly|LiveStatsPreserves)' -count=1`: tres paquetes pasan. Resultado en `ui-tests.txt`. `node --check static/js/app.js` y revisión de whitespace pasan. Un intento inicial sobre la carpeta compartida coincidió con la suite del padre y produjo diskI/O en la base de tests con nombre fijo; no se usó como resultado de producto y las verificaciones se repitieron aisladas.

**Navegador confirmado** (`ui-browser.json`):

- Guardado real: readiness cambia1/2listos50%→2/2listos100%; saved=1&missing=0 y mensaje persistente de éxito.
- Fallos de red/403/429: FormData antes/después idéntico, permanece/picks, estadoerror y botones habilitados;429 indica esperar7segundos.
- Modal: apertura dentro del diálogo, refresco realHTMX mantiene INPUT«testplayer» y selección[2,5]; Escape devuelve el foco al activadororiginal.
- Las tres procedencias muestran su etiqueta; captura final de unknown confirma ausencia de «Datos oficiales en vivo»/«ESPN FPI» (`ui-final-provenance.json`).

Pruebas adicionales de eventos y concurrencia (`ui-browser-extra.json`): cambios durante petición pendiente conservan estado unsaved sin navegación; dos rechazos específicos solo generan los dos mensajes esperados, sin toast genérico adicional.

Capturas inspeccionadas: `picks-error-mobile.png`, `picks-saved-mobile.png`, `stats-unknown-mobile.png`.

## Detector y límites

Se ejecutó una pasada mecánica Impeccable sobre templates:70señales (68warning,1error,1advisory), conservadas en `ui-detector.json`. Incluyen patrones visuales existentes —tipografíaGeist, punto animado de navegación, halos y fondo de cuadrícula—; esta tarea preserva el sistema visual y no convierte esas señales de estilo en defectos funcionales nuevos. El cambio posterior al detector ajustó únicamente textos de atribución y una aserción de prueba, tras comprobar visualmente un footer contradictorio; se recompiló el binario y repitió captura/validación de ese footer.

Las pruebas de error de guardado usan fallos controlados de fetch; las respuestas de seguridad reales se verifican en la suite del padre. La prueba de refresco usa unGET HTMX real de preparación en el mismo contenedor, no espera un partido real en vivo. No se afirma certificaciónWCAG, lector de pantalla, gesto físico ni zoom nativo. El apagado general de animaciones existente no se rediseñó.

SesiónFirefox ui-fix cerrada y servidor18112 detenido. No queda proceso de esta validación activo. Los demás agentes conservan sus archivos; no se revirtieron cambios ajenos.
