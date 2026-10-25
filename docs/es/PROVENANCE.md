# Procedencia y modificaciones de ScopeWeaver

[English](../PROVENANCE.md) · [한국어](../ko/PROVENANCE.md) · [简体中文](../zh-Hans/PROVENANCE.md) · [繁體中文](../zh-Hant/PROVENANCE.md) · [日本語](../ja/PROVENANCE.md) · Español

ScopeWeaver es una obra derivada independiente de [Autumn-27/ARTEX](https://github.com/Autumn-27/ARTEX), basada en el commit `160fe13`. Se mantiene por separado en [cskwork/scopeweaver](https://github.com/cskwork/scopeweaver), sin relación en la red de bifurcaciones de GitHub.

Las modificaciones de ScopeWeaver del **2026-10-02** abarcan el nombre del producto, interfaz y mensajes con inglés predeterminado/coreano seleccionable, documentación e indicaciones de agentes incluidas, mensajes de configuración/compilación/inicio/actualización/restablecimiento de contraseña, gestión de idioma de informes y agentes y nombres de distribución. El ejecutable es `scopeweaver` (`scopeweaver.exe` en Windows); los archivos siguen `scopeweaver-<version>-<os>-<arch>.zip`. Docker Compose compila localmente el código extraído. El flujo de versiones configurado apunta a este repositorio y GHCR.

Se conserva `LICENSE` original sin cambios. La autoría original, agradecimientos a colaboradores e historial de cambios siguen atribuidos a ARTEX. Se conservan por compatibilidad el módulo Go `github.com/Autumn-27/artex`, entrada fuente `./cmd/artex`, contratos de configuración `ARTEX_*`, valores de BD `artex` y usuario de acceso `ARTEX`.

Esta fecha de modificación no es una etiqueta de versión. No afirma que se hayan publicado binarios ni imágenes de contenedor. Los documentos históricos de validación de `sidequestion/` describen ejecuciones originales; no son comprobantes nuevos de ScopeWeaver. El JSON histórico traducido conserva identificadores y mediciones técnicas y etiqueta la narrativa traducida.

Las modificaciones del **2026-10-15** añaden formato opcional de perfil Ollama nativo (`/api/chat` con `num_ctx`, `keep_alive` y `num_predict`), tres plantillas Ollama del formulario nuevo, perfiles locales sin clave, concurrencia por perfil, valores predeterminados de precarga, compresión y juez de aprobación adaptados a Ollama y política opcional de red LLM `local_llm_only`. Se añaden variables `SCOPEWEAVER_LLM_*` y `SCOPEWEAVER_LOCAL_LLM_*` como alias con prioridad sobre los contratos `ARTEX_*` conservados. Se conserva el comportamiento de Anthropic, OpenAI, OpenAI Responses, URL personalizadas, proxies y plantillas GLM; la política está desactivada por defecto.

El candidato posterior de refuerzo nativo del **2026-10-15** añade admisión urgente al siguiente hueco sin interrupción, selección del juez según origen nativo real con salida restringida por esquema y valores de 90 segundos/preguntar, y compresión al 72% con límites de salida de herramientas de 16/32 KiB hasta 64K. Restringe los destinos estrictos opcionales a loopback y CIDR de inferencia explícitos (incluida migración LAN/Docker), rechaza redirecciones/proxies y bloquea búsqueda de agentes y acciones de red de actualización conservando estado/reversión locales. Acompañan estimaciones de memoria GPU de solo lectura, avisos de modelos compartidos, carga segura de ajustes del juez y texto inglés/coreano/chino simplificado. Son controles de aplicación, no garantías de desconexión total del host ni calidad de modelo; no introduce dependencias, migraciones, descarga automática de modelos ni versiones. Consulta la [guía de proveedores](llm-providers.md) y [verificación del candidato](VERIFICATION.md#candidato-de-refuerzo-nativo--2026-10-15).

Modificaciones adicionales del **2026-10-03** completan la auditoría de traducción del servidor y añaden plantillas GLM-5.3 con compatibilidad de razonamiento siempre activado obligatoria. Las plantillas usan el adaptador existente y no crean ni activan perfiles automáticamente.

Las correcciones posteriores originales de ARTEX de los commits `e2c8a23`, `86729b6` y
`a951e4a` se adaptan aquí: SSE de producción del mismo origen, contabilidad independiente
de tokens del modelo de aprobación y autenticación que bloquea ante fallos con creación atómica de contraseña
inicial y agrupación de conexiones de BD limitada. Los mensajes y guías de despliegue nuevos están
disponibles en inglés y coreano. La autoría original se atribuye en los commits de adaptación;
los demás cambios de ScopeWeaver permanecen.
