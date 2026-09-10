# 🚀 Guía de Despliegue en Dokploy (NFL Quiniela 2026)

Esta guía explica paso a paso cómo desplegar **NFL Quiniela 2026** en tu servidor utilizando [Dokploy](https://dokploy.com) con integración directa a tu repositorio de GitHub.

---

## 📋 Repositorio en GitHub

El código se encuentra alojado en tu cuenta de GitHub:
👉 **[https://github.com/C4risrocks/nfl-quiniela-2026](https://github.com/C4risrocks/nfl-quiniela-2026)**

---

## 🛠️ Paso a Paso para Desplegar en Dokploy

### Paso 1: Crear la Aplicación en Dokploy

1. En tu panel de Dokploy, entra al proyecto deseado y haz clic en **Create Service** $\rightarrow$ **Application**.
2. Asigna un nombre a la aplicación (ejemplo: `nfl-quiniela-2026`).

### Paso 2: Conectar el Repositorio de GitHub

1. En la pestaña **Source**:
   * **Provider**: GitHub.
   * **Repository**: Selecciona `C4risrocks/nfl-quiniela-2026`.
   * **Branch**: `main`.
   * **Build Type**: Selecciona **Dockerfile** *(Dokploy utilizará el `Dockerfile` multi-stage optimizado que no requiere instalar Go ni dependencias en el servidor)*.
2. *(Opcional)* Activa la opción **Auto Deploy / Webhook**: Cada vez que realices `git push origin main`, Dokploy compilará y actualizará la aplicación sin tiempo de inactividad.

### Paso 3: Configurar el Volumen Persistente (Imprescindible para SQLite)

Docker recrea el contenedor en cada nuevo despliegue. Para asegurar que tus usuarios, pronósticos y resultados **nunca se borren**, debes asociar un volumen persistente en Dokploy:

1. Ve a la pestaña **Volumes** (o **Storage / Almacenamiento**) de tu aplicación en Dokploy.
2. Haz clic en **Add Volume** (o **Crear Volumen**) y selecciona:
   * **Type**: `Volume` (Named Volume). *(Recomendado oficialmente por Dokploy para bases de datos).*
   * **Volume Name**: `quiniela_data`
   * **Mount Path**: `/app/data`
3. Haz clic en **Save** / **Guardar**.

> 🔒 **Permisos automáticos**: La imagen cuenta con un script de entrada (`entrypoint.sh`) que asegura automáticamente que `/app/data` pertenezca al usuario del sistema (`appuser:appgroup`), evitando cualquier error de permisos `permission denied`.
> 💡 **Copias de seguridad**: En Dokploy puedes programar respaldos automáticos a S3/R2 del volumen `quiniela_data` con un solo clic.

### Paso 4: Variables de Entorno (Environment)

En la pestaña **Environment**, pega las siguientes variables:

```env
# Puerto del contenedor
PORT=8080

# Base de datos SQLite persistente
DB_TYPE=sqlite
DB_PATH=/app/data/quiniela.db

# Clave de sesión (Genera una cadena segura de al menos 32 caracteres)
SESSION_SECRET=cambia-esta-clave-secreta-aleatoria-de-produccion-2026

# Credenciales del Administrador inicial
ADMIN_USERNAME=admin
ADMIN_PASSWORD=TuPasswordSuperSeguro2026!
ADMIN_EMAIL=tu@correo.com

# Sincronización en segundo plano con ESPN
ENABLE_BACKGROUND_SYNC=true
ESPN_SYNC_INTERVAL_MINS=5
CURRENT_SEASON_YEAR=2026

# URL pública de tu dominio (esencial para los links de verificación y recuperación)
APP_BASE_URL=https://quiniela.tudominio.com

# Servicio de Correo Gmail (SMTP con Contraseña de Aplicación de 16 letras)
# Obtén tu token en: https://myaccount.google.com/apppasswords
ENABLE_REMINDERS=true
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=tu_correo@gmail.com
SMTP_PASS=tu_token_de_16_letras_sin_espacios
SMTP_FROM=tu_correo@gmail.com
```

*(Si dejas `SMTP_HOST` vacío, el sistema opera en modo seguro "MOCK" registrando los correos en los logs sin enviar correos reales).*

### Paso 5: Dominio y Certificado SSL

1. En la pestaña **Domains**, añade tu dominio o subdominio (ejemplo: `quiniela.tudominio.com`).
2. **Container Port**: `8080`.
3. Activa la casilla **HTTPS / SSL (Let's Encrypt)**. Traefik generará y renovará automáticamente tu certificado SSL gratuito.

### Paso 6: Desplegar y Verificar
 
1. Haz clic en **Deploy** (o **Redeploy** si ya habías desplegado).
2. Dokploy compilará la imagen en 30-45 segundos.
3. Podrás verificar que la base de datos persistente está activa visitando:
   * `https://quiniela.tudominio.com/healthz`
   
   Deberás recibir un JSON confirmando el almacenamiento persistente con permisos de escritura:
   ```json
   {
     "database": "connected",
     "db_path": "/app/data/quiniela.db",
     "driver": "sqlite",
     "season": 2026,
     "status": "ok",
     "storage_writable": true
   }
   ```
   *(Si `storage_writable` es `true` y `db_path` apunta a `/app/data/quiniela.db`, tus datos están 100% a salvo y no se perderán nunca entre despliegues).*

---

## 🗄️ Alternativa: Despliegue con PostgreSQL en Dokploy

Si prefieres usar una base de datos PostgreSQL gestionada por Dokploy en vez de SQLite:

1. En tu proyecto de Dokploy, crea un servicio **Database** $\rightarrow$ **PostgreSQL**.
2. En las variables de entorno de `nfl-quiniela-2026`, reemplaza la configuración de base de datos por:
   ```env
   DB_TYPE=postgres
   DATABASE_URL=postgres://dokploy_user:tu_password@postgres-service:5432/quiniela_db?sslmode=disable
   ```
3. Al reiniciar, el sistema migrará automáticamente el esquema SQL a PostgreSQL sin necesidad de pasos manuales.
