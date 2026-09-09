# 🚀 Guía de Despliegue en Dokploy (NFL Quiniela 2026)

Esta guía explica paso a paso cómo desplegar **NFL Quiniela 2026** en tu servidor utilizando [Dokploy](https://dokploy.com).

---

## 📋 Métodos de Despliegue en Dokploy

Puedes desplegar la aplicación mediante cualquiera de estos dos métodos compatibles:

### Método A: Despliegue por Aplicación Docker / Git (Recomendado)

1. **Crear Proyecto / Aplicación en Dokploy**:
   - En tu panel de Dokploy, ve a **Projects** $\rightarrow$ **Create Service** $\rightarrow$ **Application**.
   - Asigna un nombre (ej. `nfl-quiniela-2026`).

2. **Configurar Fuente de Código (Source)**:
   - **Provider**: GitHub / GitLab / Git Repo.
   - **Repository**: Selecciona tu repositorio donde subiste este código.
   - **Branch**: `main` (o tu rama de producción).
   - **Build Type**: Selecciona **Dockerfile** (Dokploy detectará automáticamente el archivo `Dockerfile` multi-stage optimizado).

3. **Configurar Volúmenes Persistentes (Volumes)**:
   > ⚠️ **Importante para SQLite**: Para que la base de datos y los pronósticos no se borren en cada nuevo despliegue, debes montar un volumen persistente.
   - Ve a la pestaña **Volumes / Storage** de tu aplicación en Dokploy.
   - Añade un nuevo **Named Volume**:
     - **Volume Name**: `quiniela_data`
     - **Mount Path (Destino en el Contenedor)**: `/app/data`

4. **Configurar Variables de Entorno (Environment Variables)**:
   - Ve a la pestaña **Environment**:
     ```env
     PORT=8080
     DB_TYPE=sqlite
     DB_PATH=/app/data/quiniela.db
     SESSION_SECRET=tu-clave-secreta-aleatoria-de-32-caracteres
     ADMIN_USERNAME=admin
     ADMIN_PASSWORD=TuContraseñaSeguraAdmin2026!
     ADMIN_EMAIL=tu@correo.com
     ENABLE_BACKGROUND_SYNC=true
     ESPN_SYNC_INTERVAL_MINS=5
     CURRENT_SEASON_YEAR=2026
     ```

5. **Configurar Dominio y SSL**:
   - En la pestaña **Domains**, añade tu dominio (ej. `quiniela.tudominio.com`).
   - **Container Port**: `8080`.
   - Activa el certificado **HTTPS / Let's Encrypt**.

6. **Desplegar**:
   - Haz clic en **Deploy**. Dokploy compilará el binario en Go y pondrá la aplicación en producción con certificado SSL automático y monitoreo `/healthz`.

---

### Método B: Despliegue por Docker Compose

Si prefieres gestionar la aplicación mediante Compose en Dokploy:

1. Ve a **Projects** $\rightarrow$ **Create Service** $\rightarrow$ **Compose**.
2. Dokploy detectará el archivo `docker-compose.yml` incluido en este repositorio.
3. En la pestaña de variables de entorno de Dokploy, define tus credenciales de producción.
4. Haz clic en **Deploy**. El volumen con nombre `quiniela_data` se creará automáticamente y permitirá respaldos automáticos S3 desde la interfaz de Dokploy.

---

## 🗄️ Opción: Conexión a PostgreSQL en Dokploy

Si deseas utilizar PostgreSQL en lugar de SQLite:

1. En Dokploy, crea un servicio de base de datos **PostgreSQL** (ej. `postgres-service`).
2. En las variables de entorno de `nfl-quiniela`, ajusta:
   ```env
   DB_TYPE=postgres
   DATABASE_URL=postgres://dokploy_user:tu_password@postgres-service:5432/quiniela_db?sslmode=disable
   ```
3. Reinicia la aplicación. El sistema creará automáticamente todas las tablas y datos semilla en PostgreSQL.

---

## 🛡️ Copias de Seguridad (Backups)

- Dokploy incluye soporte nativo para **Volume Backups** hacia destinos compatibles con S3 (AWS S3, Cloudflare R2, MinIO, Wasabi, etc.).
- Configura una tarea programada en Dokploy para respaldar el volumen `quiniela_data` diariamente durante la temporada.
