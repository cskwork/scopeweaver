# Verificación y limitaciones conocidas

[English](../VERIFICATION.md) · [한국어](../ko/VERIFICATION.md) · [简体中文](../zh-Hans/VERIFICATION.md) · [繁體中文](../zh-Hant/VERIFICATION.md) · [日本語](../ja/VERIFICATION.md) · Español

## Correcciones de ejecución compartida desde verificación CLI independiente (2026-10-17)

Rama candidata `fix/cli-discovered-runtime-bugs`, basada en el commit `dev` `3f0ef25`. Son resultados locales; las comprobaciones alojadas de la revisión exacta de la PR son independientes. Este cambio no añade la CLI independiente ni proveedores por suscripción a la aplicación original.

- Las herramientas HTTP personalizadas ahora usan la URL completa del proxy de grabación devuelta por `Manager.ProxyAddr`. Antes: `"http://" + addr` convertía `http://127.0.0.1:<port>` del caso controlado en `http://http://127.0.0.1:<port>`, por lo que fallaba antes de llegar al proxy. Después: `proxyStr = addr` conserva la URL. Pasan las rutas reales directa, explícita y automática; la automática fallaba antes de corregir.
- El inicio comprueba ahora que una ruta de transcripción existente sea un directorio antes de leerla. La regresión de archivo normal en Windows fallaba antes; ahora rechaza la ruta conservando el archivo. Los directorios ausentes siguen siendo válidos. El diagnóstico nuevo está en inglés, coreano y chino simplificado.
- El apagado cancela trabajo, vacía HTTP y espera chats admitidos, respuestas paralelas y publicación de puntos de control antes de cerrar almacenes. Antes, vaciar solo HTTP podía dejar finalizadores en segundo plano escribiendo tras cerrar la BD. Los errores finales de escritura de instantánea/contexto se propagan, conservan pendientes y hacen fallar explícitamente el apagado; registros eliminados/en conflicto siguen siendo inocuos. Este vaciado limitado no afirma cubrir cada planificador o productor del motor.
- Los datos de la prueba de uso cancelado crean ahora la tabla opcional de uso existente, como la inicialización de producción. El fallo por tabla ausente se reprodujo antes de corregir los datos; la prueba corregida con BD nueva conserva la aserción exacta de uso.

Las comprobaciones reales de apagado PostgreSQL pasaron 11 pruebas/subpruebas, incluidas persistencia tras cerrar/reabrir y fallos finales de escritura inyectados. Las del proceso principal pasaron 4 pruebas, cubriendo salida normal 0, reinicio 75, fallos limitados y limpieza ante fallo real de servicio por puerto ocupado. Pasaron regresiones específicas de proxy/inicio/idiomas. La ejecución completa del backend en Windows tuvo 1,975 aprobadas, 2 fallidas y 15 omitidas: un fallo era una limitación de privilegios sin cambios al crear enlaces simbólicos; el otro reveló el recuento esperado del catálogo, actualizado de 1,980 a 1,982 por los dos diagnósticos nuevos sin quitar comprobaciones de integridad. El catálogo corregido se verifica por separado. Las comprobaciones Linux/alojadas deben establecer los resultados restantes.

Frontend: 65 pruebas aprobadas y exportación estática correcta con Git Bash para el script npm de estilo POSIX existente. Adaptador de agentes: 7 pruebas y comprobaciones de sintaxis aprobadas. Análisis estático específico del backend correcto. No se usaron BD de usuario, objetivos reales en línea ni entregas de notificaciones; cada ejecución que escribía en BD creó y eliminó una `scopeweaver_test_*` independiente. No se afirma integración ni publicación de versión.

## Controles de chat y contexto (2026-10-16)

El autor de la [PR #29](https://github.com/cskwork/scopeweaver/pull/29) comunicó estas comprobaciones sobre `6366429` original. Son sus resultados históricos de Linux, anteriores a la revisión conjunta de PR #29/#30; no establecen el estado del candidato posterior ni la copia de adjuntos en Windows.

| Comprobación | Resultado |
| --- | --- |
| Suites de carreras con PostgreSQL 17 desechable (`db`, `server`, `agent`) | 667 pruebas de nivel superior aprobadas; sin carreras detectadas |
| Paquetes Go y análisis estático | `go test ./... -count=1` y `go vet ./...` correctos |
| Pruebas frontend, traducciones, auditoría y exportación estática | 65 aprobadas; 3,263 claves EN/KO/zh-Hans pasaron paridad; 0 hallazgos de auditoría; TypeScript y 31 páginas correctos |
| Traducciones backend | 1,978 claves de producción pasaron integridad, paridad y printf en tres idiomas |
| Control de navegador de chat | 60 escenarios aprobados en EN/KO/zh-Hans, 1440/390 px y chat independiente/de tarea; sin errores de página no capturados |
| Prueba básica y panel con aplicación integrada | E2E básica y 14 escenarios de panel aprobados; sin errores de navegador no capturados |
| Adaptador de agentes | 7 pruebas unitarias, sintaxis y 3 escenarios MCP/Pi aprobados |
| Instalador y empaquetado | 8 casos de instalador, 11 Compose, 5 archivos de empaquetado por idioma (EN/KO) y 6 pruebas de avisos de versión aprobados |

Los casos de navegador de chat cubren edición y orden de cola, reconocimiento de reinicio, reintento/edición/ramificación, inspección y compresión de contexto, corrección de herramientas y pegado de documentos. Las regresiones backend cubren persistencia del idioma aceptado, errores de comprobación previa, intercambios de herramientas interrumpidos, preparaciones de rama abandonadas y atribución de lectura de documentos.

Puedes inspeccionar el contexto actual del modelo separado del uso facturado de conversación. La compresión conserva los intercambios recientes completados que elijas; mantienes todo el historial visible y el uso de intentos anteriores.

El colaborador informó de una revisión que encontró seis defectos de integración, seguidos de correcciones de admisión de entradas generadas y recuperación de comprobaciones, rutas de adjuntos tras compresión, historial visible antiguo y controles de entrega de correcciones. La prueba de adjuntos elimina la conversación principal antes de las siguientes instrucciones de la hija; otra comprueba que la reasignación conserva el texto literal del manifiesto. Informó de una regresión fallida antes de cada corrección y de una repetición local completa. La aserción del total de razonamiento del panel se limitó a la fila de resumen porque el texto emergente usa la misma etiqueta; la repetición comunicada pasó 14 escenarios de panel.

Las pruebas de inferencia comunicadas usaron modelos loopback y BD desechables. La ejecución Windows, proveedores de pago y CI remota requieren verificación aparte. El colaborador usó Python 3 para el caso de avisos, como exige CI.

## Candidato de refuerzo nativo — 2026-10-15

Comprobaciones locales e independientes de `feat/ollama-zero-egress-hardening`, basado en main `33b131e`. Son resultados del candidato; CI remota de revisión exacta y entrega de PR siguen pendientes. Las secciones históricas siguientes conservan sus resultados originales.

| Comprobación realmente ejecutada | Resultado |
| --- | --- |
| Ejecución Go independiente, 20 paquetes con BD PostgreSQL desechables aisladas | 1,366 pruebas/subpruebas aprobadas, 0 fallos; `enrich` no tiene pruebas; 2 omisiones opcionales (`TestAgentAdapterE2E`, `TestLiveContextReview`) |
| Adaptador de agentes, comprobado aparte | 7 pruebas unitarias y 3 escenarios reales MCP/Pi aprobados; revisión de proveedor externo real no ejecutada intencionadamente |
| Suites específicas backend `-race` | Correctas sin carreras: transporte, protocolo/uso, contexto, selección/fallo de origen, búsqueda/actualización y admisión |
| `go vet ./...` | Correcto |
| Pruebas unitarias/localización frontend y exportación estática | 45 aprobadas; 3,168 claves EN/KO/zh-Hans conservan paridad, marcadores y etiquetas; TypeScript y las 31 páginas estáticas correctos |
| Catálogo backend | 1,943 claves de producción pasaron integridad/paridad directa en tres idiomas y printf |
| Instalador, Compose y avisos | 8 casos de instalador, 11 de credenciales Compose, avisos en 5 archivos de empaquetado por idioma (EN/KO) y 6 pruebas de avisos de versión bajo Python 3.12 aprobados |
| Compilaciones estáticas y dependencias | Compilación integrada macOS y cruzada Windows amd64 correctas; sin cambios de `go.mod`/`go.sum`; ejecución Windows nativa no realizada |
| Prueba básica de navegador integrado | Interfaz final recompilada: 60 aprobadas (48 básicas + 12 de panel), 0 fallos. Capturas asentadas escritorio/móvil y límites horizontales correctos en EN/KO/zh-Hans: panel [0,390] a 390px y [864,1440] a 1440px; los 20 contenidos críticos caben. Se reprodujo y corrigió el recorte previo por ancho mínimo de 420px |

La [regresión de prioridad HTTP nativa](../../server/ollama_priority_test.go) retiene al trabajador 1 dentro de `/api/chat`, pone en cola trabajadores 2/3 y después invoca el juez real conectado sin marcador de urgencia del llamante. Pasó con `-race` con orden HTTP **trabajador 1 → juez → trabajador 2 → trabajador 3**, concurrencia máxima 1, esquema/veredicto nativo válido, todas las llamadas terminadas y sin fuga de huecos. Las pruebas de cola existentes cubren roles FIFO, cancelación y reducción de límites.

La [regresión de fallo de origen nativo](../../server/provider_origin_failure_test.go) falló antes de corregir: una herramienta nativa pendiente seguida de error de transmisión podía heredar `allow` global de nube. Tras corregir, los errores de enrutamiento Stream/Complete y finales conservan el perfil nativo concreto, política del modelo de 90 segundos/`ask` y caducidad humana de 300 segundos/`deny`, sin llamar al revisor. La conmutación real a nube sí borra la preferencia nativa. La regresión y las suites de carreras afectadas pasaron.

Las comprobaciones reales de API autenticada alternaron modo estricto **false → true → false** contra un proxy local que nunca reenvía solicitudes. Los cuatro motores (`ddgs`, `brave-free`, `tavily`, `deepseek`) llegaron a ese receptor solo con modo estricto desactivado; cada uno produjo cero solicitudes adicionales en modo estricto. Se bloquearon comprobaciones y aplicación de actualizaciones estrictas conservando estado local. Las [regresiones de actualización](../../server/update_policy_test.go) también pasaron para caché, redirecciones, lectura de cuerpos en directo y limpieza/finalización de fallos tras bloquear una descarga asíncrona admitida con un cambio posterior. No demuestran control de salida de todo el host.

Ollama **0.32.15** del host, con `qwen3:4b` existente y **presupuesto de salida de 4,096 tokens**, pasó diagnósticos nativos, esquema decision/comment, repetición estructurada de llamada/resultado de herramienta, uso y contexto solicitado de 32K (`/api/ps`: **32,768**). Los fallos anteriores con presupuestos de 256/512 tokens siguen en las pruebas; no se descartaron como ejecuciones correctas. Los valores de carga/velocidad/descarga son una instantánea en caliente, no una prueba comparativa ni garantía de permanencia en memoria. No se descargaron modelos ni cambió configuración del host; no se afirma finalización autónoma ni calidad de evaluación de seguridad.

La cobertura del navegador incluye diagnósticos/avisos en tres idiomas, estado de actualización estricta, protección de carga/fallo del juez y flujos existentes de panel/uso. La revisión visual reprodujo un editor de ancho mínimo fijo de 420px que sobresalía 30px a la izquierda de una pantalla de 390px. El mínimo ahora cabe; la aplicación recompilada pasó las 60 comprobaciones y las seis capturas de escritorio/móvil muestran etiquetas legibles y contenido asentado dentro de la pantalla. Una recompilación integrada independiente coincidió byte a byte con el binario del navegador. CI remota de revisión exacta y lectura posterior de PR son controles de entrega separados. No se afirma integración, versión, llamada de proveedor de pago, escaneo externo ni notificación real.

## Seguimiento de autenticación de proveedores — 2026-10-15

Las regresiones locales en `fix/provider-env-auth`, basado en main integrado `38fde23`, reproducen y corrigen dos fallos de pasarelas locales autenticadas. La alternativa de clave al iniciar y la prueba de conexión sin clave devolvían cada una HTTP 401 con la implementación anterior para Anthropic, OpenAI Chat Completions y OpenAI Responses. Los mismos casos pasan con las correcciones.

| Comprobación | Resultado |
| --- | --- |
| [Autenticación HTTP local `FromEnv → NewProvider`](../../agent/provider_env_test.go) | 21 escenarios aprobados: 18 de formato genérico sobre claves de proveedor/archivo, sustituciones modernas/antiguas y vacías explícitas; 3 nativos sobre aislamiento de claves de nube, token Ollama y sustitución vacía explícita |
| [Controlador de prueba de conexión con casos HTTP locales](../../server/llm_gateway_test.go) | 16 escenarios aprobados: 15 genéricos sobre servicio normalizado coincidente, endpoint/formato/proxy diferentes y alternativa no local conservada; 1 nativo que rechaza heredar clave de nube activa |
| Ambos grupos específicos con `-race` | Correctos, sin carreras |

Los **37 casos** anteriores cuentan escenarios hoja, excluyendo pruebas principales. Los fallos de autenticación se reprodujeron antes de cada corrección y las mismas pruebas pasaron después. Las direcciones públicas de documentación de casos no locales usan un proxy local explícito; no se contacta a proveedores públicos.

Una prueba de conexión sin clave ni ID de perfil puede reutilizar la clave del servicio local activo solo si coinciden formato genérico, URL base normalizada y proxy, incluso en un borrador nuevo. Los servicios locales diferentes y Ollama nativo permanecen aislados. Guardar un perfil nuevo sin clave no importa credenciales del entorno.

Estas comprobaciones establecen los límites de protocolo y credenciales probados, no disponibilidad de cuentas de pago ni todas las funciones históricas. Los resultados de suite completa, navegador, CI, main integrado y artefactos de versión posteriores son controles separados. No se hicieron llamadas a proveedores de pago.

## Seguimiento de selección de modelos instalados — 2026-10-15

Verificación local de selección Ollama predeterminada de perfiles nuevos en `feat/ollama-native-flexible-providers`. Sustituye el comportamiento anterior de plantillas: elegir Ollama ahora realiza descubrimiento de modelos de solo lectura. La instantánea histórica de 37 comprobaciones de navegador siguiente es anterior; su aserción «sin solicitudes implícitas» es histórica, no el contrato actual de Ollama.

| Comprobación | Resultado |
| --- | --- |
| Pruebas unitarias y localización frontend (`npm --prefix web test`) | 45 aprobadas, 0 fallos |
| Exportación estática de producción (`npm --prefix web run build:static`) | Correcta, incluido TypeScript y las 31 páginas |
| Descubrimiento real de modelos del host mediante navegador y backend autenticado | Correcto en perfil nuevo sin guardar, Ollama en mismo equipo, proxy y clave API vacíos |
| Prueba básica completa ampliada en binario integrado y BD nueva `scopeweaver_test_default_final` | 43 aprobadas, 0 fallos (31 básicas + 12 de panel). Cubre valores instalados EN/KO en las tres plantillas, formato directo, compatibilidad de nube, selección solo de metadatos, detección fallida/vacía, edición manual/rechazo de respuestas tardías, reapertura de borradores/perfiles existentes y regresiones de configuración, acceso, localización, SSE y panel |

`/api/tags` del host real devolvió `qwen2.5-coder:7b`, `qwen3:4b` y `embeddinggemma:latest`. El navegador seleccionó **`qwen2.5-coder:7b`**, primer modelo instalado devuelto que declara `tools`. Sus metadatos indicaron `completion`, `tools`, `insert`, 7.6B parámetros y cuantización Q4_K_M. `qwen3:4b` también declaró `tools`; `embeddinggemma:latest` solo `embedding`.

El navegador envió exactamente una operación LLM, `POST /api/llm/models`, al endpoint del host sin clave ni proxy. El borrador nuevo se cerró sin guardar, activar, probar conexión ni inferir. No cambiaron la configuración de modelos ni del equipo. Esto demuestra selección y descubrimiento de metadatos, no ejecución de herramientas ni fiabilidad autónoma.

## Rama histórica de Ollama nativo — 2026-10-15 (local a la rama, no una versión)

Comprobaciones locales de `feat/ollama-native-flexible-providers`, registradas antes de abrir la PR. La CI remota se sigue por separado en GitHub Actions; nada de esto es un resultado de publicación, integración ni CI.

| Comprobación | Resultado |
| --- | --- |
| Suites Go, 20 paquetes ejecutados por separado, cada uno con BD PostgreSQL 16 desechable nueva | 1,197 pruebas/subpruebas aprobadas; 2 omisiones opcionales (`TestAgentAdapterE2E`, `TestLiveContextReview`) |
| Adaptador de agentes, ejecutado aparte | 3 escenarios reales MCP/Pi aprobados |
| `go vet ./...` | Correcto |
| Pruebas unitarias y localización frontend | 43 aprobadas; 3,159 claves únicas en inglés, coreano y chino simplificado |
| Catálogo de mensajes backend | 1,938 claves únicas en cada idioma; printf y paridad correctos |
| TypeScript y exportación estática de producción | Correctos, recompilados tras cambios finales de interfaz de estado |
| Auditoría de dependencias frontend (`npm audit`) | 0 vulnerabilidades |
| Compilaciones | Binario integrado `CGO_ENABLED=0` recompilado tras cambios finales de estado; compilación cruzada Windows amd64 `CGO_ENABLED=0` correcta (ejecución Windows nativa no realizada) |
| Prueba básica de navegador (`npm run smoke`) con binario integrado real y BD nueva `scopeweaver_test_ollama_final_health` | 37 aprobadas, 0 fallos (25 básicas + 12 de panel). Cubre tres plantillas Ollama EN/KO sin solicitudes implícitas, guardar/activar sin clave, estado de compatible local sin clave frente a nube pública sin clave, formatos de nube y GLM editable, metadatos/avisos de herramientas por modelo, comando exacto de descarga de modelo ausente, diagnósticos de herramienta/contexto/carga/velocidad y política exclusivamente local desactivada por defecto, además de regresiones EN/KO/zh-Hans de configuración, acceso, idioma, borradores, SSE y uso de aprobación |
| Credenciales Compose | 11 casos aprobados en contenedores reales `network_mode: none`, mediante entorno del contenedor y `FromEnv` |
| Instalador y empaquetado | 8 casos de instalador aprobados en total, incluida selección inglesa/coreana; 5 archivos aprobados en cada idioma |
| Avisos de versión (Python 3.12) | 6 pruebas aprobadas |
| Revisión independiente de seguridad/corrección backend | Correcta, incluidas política exclusivamente local DeepSeek, comprobaciones CIDR en directo y clasificación 404 |

### Ejecución real de Ollama

Ollama 0.32.15 del host con `qwen3:4b`, mediante API nativa en transmisión:

- En una comprobación de transmisión del SDK con una herramienta sintética, el modelo llamó a `record_fact`; ScopeWeaver la ejecutó localmente, repitió el resultado y el turno terminó. No era un `record_fact` de la BD de plataforma.
- Se solicitaron 32K de contexto; `/api/ps` indicó 32,768 asignados.
- Uso exacto de esa comprobación: turno en transmisión 186 tokens de entrada / 750 de salida; turno final 826 de entrada / 193 de salida.
- Guardar/activar sin clave mediante API real fue correcto. Los diagnósticos indicaron `tool_call_verified` y `context_verified` true, contexto 32,768, carga 2.225375 ms, 29.231 tokens/s y latencia 18,958 ms. **Fue una ejecución en caliente, no una comparación de arranque en frío ni generalización de hardware.**
- Pasaron la indicación coreana exacta de descarga para modelo ausente y el interruptor estricto que bloquea una IP pública.
- **Prueba Bash de trabajador de tarea de plataforma:** un trabajador real (tarea 1) con `qwen3:4b` (32K solicitados, 32,768 asignados; concurrencia 1; keep-alive 30m; pensamiento desactivado; máximo 4,096 tokens) hizo la llamada Bash nativa `call_219yv72x`. Las actividades 9 (llamada) y 10 (`tool_result`) comparten ese ID sin error y la salida contenía `LOCAL_TASK_OLLAMA_OK`. El registro contabilizó exactamente 6,704 tokens de entrada / 4,096 de salida y la siguiente llamada nativa repitió el mismo `tool_call_id`. La intención se creó manualmente con `db.AddIntent`, no mediante LLM. La salida también contiene un `\n` literal por el escape del caso; no se infiere un problema de saltos de línea ni del modelo. Se usó un binario compilado del código sin confirmar de la rama anterior a los cambios finales de interfaz/DTO.

### La fiabilidad de tareas autónomas no está validada

Un diagnóstico de llamadas a herramientas correcto demuestra la ruta de protocolo, no que el modelo complete tareas.

- `qwen2.5-coder:7b` declara `tools`, pero escribió JSON con forma de herramienta en `assistant.content` en vez de `tool_calls` del protocolo, por lo que no se ejecutó Bash ni herramientas de hechos. ScopeWeaver lo conservó correctamente como texto.
- `qwen3:4b` como agente principal, ante instrucciones largas, deliberó y escribió el marcador esperado como texto. Como trabajador sí ejecutó Bash (arriba), pero la tarea se pausó a los 360 s sin `record_fact` ni finalización autónoma; una llamada correcta no demuestra finalización autónoma.

**No se afirma finalización autónoma de extremo a extremo ni calidad de evaluación de seguridad para ningún modelo local, y los modelos de clase 7B no están validados como agente completo.** Los [niveles de hardware](llm-providers.md#niveles-de-hardware) son estimaciones, no comparaciones medidas.

### No probado

Sin llamadas de nube de pago, escaneos externos, notificaciones reales, publicación, despliegue, versiones ni integración. Anthropic, OpenAI Chat/Responses y pasarelas personalizadas se verificaron mediante contratos de protocolo, regresiones y flujos editables de navegador, no cuentas reales. Todas las BD fueron desechables y no cambió la configuración Ollama del host. La política opcional `local_llm_only` está desactivada por defecto; los servidores aún deben establecer `OLLAMA_NO_CLOUD` aparte.

## Ejecutar pruebas con base de datos

Las suites `db`, `server` y `agent` necesitan PostgreSQL. Resuelven su
cadena de conexión **solo** desde la variable de entorno `ARTEX_TEST_DSN` y
nunca usan como alternativa el DSN de la aplicación (`ARTEX_PG_DSN` /
`config.json`). Es intencionado: evita que `go test` modifique la base de datos
activa/de producción de la aplicación.

- **Sin `ARTEX_TEST_DSN`** → se omiten todas las pruebas con BD. No se toca ninguna base
  de datos.
- **`ARTEX_TEST_DSN` con BD desechable** → las suites se ejecutan contra ella.
- **`ARTEX_TEST_DSN` resuelve a la misma BD que la aplicación** (mismo host,
  puerto y nombre) → la ejecución falla inmediatamente con
  `refusing to run tests against the live database; point ARTEX_TEST_DSN at a
  throwaway DB`, antes de que se conecte ninguna prueba.

La forma más sencilla de ejecutarlas es:

```bash
make test-db
```

`make test-db` (mediante `scripts/test-db.sh`) crea una BD desechable llamada
`scopeweaver_test_<pid>` propiedad del rol `sw_user_test`, dirige
`ARTEX_TEST_DSN` a ella, ejecuta `go test ./db/ ./server/ ./agent/ -count=1` y
elimina la BD al salir, incluso si fallan las pruebas. Sustituye `TEST_DB_ROLE`,
`TEST_DB_HOST` o `TEST_DB_NAME` según tu PostgreSQL local. El nombre de BD
debe empezar por `scopeweaver_test_` (o `scopeweaver_adapter_` para la
prueba E2E del adaptador de agentes).

Para ejecutar un subconjunto contra una BD gestionada por ti, exporta `ARTEX_TEST_DSN`
y llama directamente a `go test`:

```bash
export ARTEX_TEST_DSN='postgres://sw_user_test@/scopeweaver_test_1?host=/tmp&sslmode=disable'
go test ./server/ -run TestSomething -count=1
```

## Candidato v0.2.1 — 2026-10-11

El candidato de versión Windows se compiló con el frontend estático real y se probó contra BD PostgreSQL 16.15 desechables. Estos resultados sustituyen las limitaciones históricas de dependencias y ejecución Windows siguientes; el registro anterior se conserva para trazabilidad.

| Comprobación | Resultado |
| --- | --- |
| Auditoría de dependencias frontend | 0 vulnerabilidades de cualquier gravedad, incluidas dependencias de desarrollo |
| Pruebas de regresión y localización frontend | 26 aprobadas; sin omisiones |
| Exportación estática de producción | TypeScript correcto; 31 páginas generadas |
| Suites Go nativas Windows, 18 paquetes | 1,016 pruebas/subpruebas aprobadas; 0 fallos; 13 omisiones existentes de plataforma/opcionales |
| Suite completa de servidor Windows nativo | 311 pruebas/subpruebas aprobadas; 3 omisiones existentes opcionales/de plataforma |
| Adaptador de agentes | 7 pruebas unitarias y sintaxis correctas; 3 escenarios MCP/Pi aprobados contra backend real autenticado y BD nueva |
| Instalador local | Los 4 casos aislados aprobados: instalación nueva, sustitución de archivo obsoleto, fallo de dependencia y fallo de compilación frontend |
| Catálogo de traducción backend | 1,868 claves de producción pasaron integridad inglés/coreano y argumentos printf |
| Aislamiento de notificaciones | La regresión determinista de trabajador obsoleto falló antes de corregir y pasó después; selección de gestión/notificación pasó 5 ejecuciones consecutivas |
| Pruebas Windows | Suite completa con PostgreSQL correcta, incluidas escrituras simultáneas, fallos de confirmación y rutas largas |
| Python y archivos de habilidades Windows | Ejecución Python real y alternativa a iniciador inutilizable correctas; rutas Unicode anidadas pasaron por listado y controladores de lectura |
| E2E de navegador en binario Windows integrado | 8 comprobaciones aprobadas: protección ante caída de configuración en ambos idiomas, configuración/acceso, SSE del mismo origen, aislamiento de API de uso, ambas tarjetas de uso localizadas y sin errores de página no capturados |

Los recuentos Go incluyen pruebas principales y subpruebas. Las 13 omisiones nativas incluyen diez casos Unix de actualización automática, un privilegio de enlace simbólico no disponible, la integración opcional del adaptador y el proveedor real. La integración se comprueba por separado y CI Linux cubre casos Unix. No se desactivaron pruebas para obtener estos resultados.

Las regresiones reproducen el fallo original Windows de sincronización de directorios, diferencia de separador de localización, iniciador Python inutilizable, fuga de trabajador de notificaciones y orden de limpieza de perfiles. El auxiliar SQL E2E ahora coloca opciones antes de la URL de conexión; una comprobación directa de caída/restauración confirmó que psql de Windows ejecuta las sentencias.

El control de versión exige además trabajos `frontend`, `windows`, `backend`, `agent-adapter` y `smoke` de navegador antes y después de integrar. Los resultados de publicación, archivos de plataforma y sumas están en la [versión v0.2.1](https://github.com/cskwork/scopeweaver/releases/tag/v0.2.1); este registro fuente no afirma que los trabajos posteriores o recursos descargados ya hayan pasado.

Se conservan las rutas existentes de intérprete Python. Si una instalación anterior guardó un alias de iniciador inutilizable, elige un intérprete ejecutable en Ajustes. Las pruebas Windows nativas no establecen actualización automática Windows ni ejecución macOS. No se usaron proveedores reales de pago, objetivos externos ni destinatarios reales de notificaciones.

## Verificación histórica — 2026-10-03

Validación registrada el **2026-10-03** para la edición inglés/coreano de ScopeWeaver, basada en el commit ARTEX original `160fe13`. Las pruebas usan BD PostgreSQL desechables y casos locales. Es entrega de código fuente; no se han publicado binarios ni imágenes de contenedor.

| Comprobación | Resultado |
| --- | --- |
| Pruebas de regresión y localización frontend | 21 aprobadas; TypeScript correcto |
| Exportación estática de producción y binario Go integrado | Compilados correctamente |
| Suites de regresión Go, 18 paquetes | 986 pruebas/subpruebas aprobadas; 1 heredada falló; 1 de proveedor externo omitida |
| Suite completa de servidor | 285 pruebas/subpruebas aprobadas; solo se omitió la prueba opcional de proveedor real |
| Catálogo de traducción backend | 1,865 claves de producción pasaron integridad inglés/coreano y argumentos printf |
| Auditoría de texto chino redactado | Sin código/documentación redactados ni comentarios Go sin traducir; excepciones deliberadas de compatibilidad/datos conservadas |
| Cobertura de navegador de escritorio | 24 rutas por idioma; sin etiquetas ausentes ni desbordamiento horizontal de página |
| Cobertura de navegador móvil | Panel, tareas, hallazgos y ajustes en ambos idiomas; sin desbordamiento horizontal de página a 390px |
| Pantallas reales de aplicación | Formularios de acceso y notificaciones comprobados en ambos idiomas; validación e inicialización verificadas |
| Cambio de idioma | Etiquetas de navegación/estado se actualizan; selección y borradores sin enviar conservados |
| Localización HTTP | Exportación Markdown/CSV, negociación de idioma, preferencia guardada y rechazo de idioma no válido comprobados |
| Contrato de proveedor GLM-5.3 | 21 pruebas/subpruebas específicas aprobadas con HTTP/SSE local; cubiertas ambas rutas de endpoint, razonamiento y repetición de llamadas a herramientas |
| Flujo de plantillas GLM en navegador | Ambas plantillas comprobadas en ambos idiomas; nombre/clave/proxy introducidos conservados; sin guardar, activar ni solicitar al proveedor |
| Scripts de empaquetado e inicio | Sintaxis shell, casos bilingües de empaquetado y 10 casos de supervisor aprobados |
| Procedencia y dependencias | Licencia original sin cambios; versiones de dependencias y módulo Go sin cambios; enlaces de documentación y diferencias correctos |

Los recuentos incluyen pruebas Go principales y subpruebas. Las ejecuciones de paquetes usan BD independientes inicializadas con las mismas tablas adicionales de medición que el inicio normal del servidor. Las pruebas y comprobaciones originales siguen activadas.

### Problemas heredados entonces

`TestGraphOverviewExpandsAssociatedCompanyScope` falló con `task scope missing: <nil>` tanto en ScopeWeaver como en el código original intacto. Esperaba alcance detallado en el resumen del grafo, que el commit original `06a43f3` hizo compacto deliberadamente. El commit `75a706d` lo sustituyó por `TestGraphOverviewKeepsAssociatedCompanyScopeCompact`, que comprueba el resumen compacto y verifica directamente el alcance almacenado de empresa. La CI completa del backend ha pasado desde entonces.

`TestTaskMetadataPatchReturnsRenameAndPin` produjo antes un fallo intermitente de limpieza de directorio temporal en la verificación original. Pasó en la ejecución final del servidor ScopeWeaver; no se hizo un cambio ajeno de ciclo de vida para ocultarlo.

La vista previa simulada original no tiene simulación de metadatos de notificación, por lo que su página Notificaciones falla. La **página real de Notificaciones del backend y el formulario de añadir canal pasan** en ambos idiomas. No se envió ninguna notificación al verificar.

La verificación v0.1.0 registró **14 hallazgos de npm audit: 1 crítico, 9 altos y 4 moderados**. Actualizaciones de framework/herramientas y eliminación de la dependencia CLI shadcn sin usar resolvieron los hallazgos en el candidato v0.2.1 anterior. Ejecuta `cd web && npm audit` para el informe actual del registro; puede cambiar al publicarse nuevos avisos.

### Alcance y límites históricos

- `TestLiveContextReview` opcional requiere configuración privada de proveedor externo y no se ejecutó. No hubo llamadas a modelos de pago, escaneos externos ni entregas reales de notificaciones.
- Las pruebas GLM demuestran el contrato local de solicitud/respuesta, no acceso a cuentas ni disponibilidad del proveedor. Coding Plan se restringe a herramientas oficialmente compatibles y ScopeWeaver no está en la lista; consulta [configuración de proveedores](llm-providers.md).
- No se verificaron ejecución de imagen Docker ni archivos por lotes Windows. Los casos de empaquetado no afirman pruebas reales de ejecución Windows.
- La configuración llegó a la ruta autenticada, pero una espera de navegador del evento de carga de ventana agotó el tiempo; después se confirmaron la inicialización y contraseña nueva mediante API real.
- El texto introducido por usuarios, pruebas guardadas, instrucciones editadas, identificadores de protocolo, marcadores antiguos de interpretación, casos Unicode y capturas originales conservan su contenido. La aplicación inglés/coreano no traduce datos arbitrarios del usuario ni salida de herramientas de terceros.
- Los documentos históricos de validación de `sidequestion/` describen trabajo original, no ejecuciones nuevas de ScopeWeaver.

Las comprobaciones remotas actuales están en [GitHub Actions](https://github.com/cskwork/scopeweaver/actions/workflows/ci.yml).

## Pegado de documentos en chat (2026-10-16)

La PR #29 registra las siguientes comprobaciones de pegado con BD PostgreSQL 17 desechable y backend local. Son resultados comunicados por el colaborador del 2026-10-16; las pruebas de integración actuales se registran aparte:

- La suite frontend pasó 52 pruebas; la exportación estática de producción pasó.
- El navegador pasó 12 combinaciones de portapapeles: chat independiente/de tarea, EN/KO/zh-Hans y 1440/390 px.
- Los casos cubrieron pegados rápidos de documentos con un propietario de borrador, texto acompañante, inserción nativa de 3,000 caracteres y deshacer, vista previa Unicode, restauración de borrador, reintento de subida, navegación, menciones, IME y ambas preferencias de envío. Pegar no envió mensajes.
- El colaborador desactivó el controlador React de pegado para confirmar que fallaba la comprobación por ausencia del segundo indicador de adjunto; lo restauró y repitió.
- La suite básica backend pasó portapapeles y casos Ollama loopback sin errores de página no capturados.

Los casos de archivos proporcionan objetos File del navegador mediante eventos de pegado. Los de texto y deshacer usan el portapapeles Chromium y pegado por teclado.
