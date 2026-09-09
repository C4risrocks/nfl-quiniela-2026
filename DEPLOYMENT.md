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

Para asegurar que la base de datos de los pronósticos y usuarios nunca se borre al actualizar la versión:
1. Ve a la pestaña **Volumes** (o **Storage**) de la aplicación en Dokploy.
2. Añade un **Named Volume**:
   * **Type**: Named Volume.
   * **Volume Name**: `quiniela_data`
   * **Mount Path**: `/app/data`

> 💡 **Tip Dokploy**: Dokploy permite programar copias de seguridad automáticas (S3/Cloudflare R2/Wasabi) del volumen `quiniela_data` con un solo clic.

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

# URL pública de tu dominio (para los enlaces de los correos de recordatorio)
APP_BASE_URL=https://quiniela.tudominio.com

# Notificaciones y Recordatorios por Correo (Opcional - SMTP estándar)
ENABLE_REMINDERS=true
SMTP_HOST=smtp.resend.com
SMTP_PORT=587
SMTP_USER=resend
SMTP_PASS=re_tu_api_key_aqui
SMTP_FROM=quiniela@tudominio.com
```

*(Si dejas `SMTP_HOST` vacío, el sistema continuará funcionando normalmente y registrará los recordatorios en modo simulación/registro en la consola).*

### Paso 5: Dominio y Certificado SSL

1. En la pestaña **Domains**, añade tu dominio o subdominio (ejemplo: `quiniela.tudominio.com`).
2. **Container Port**: `8080`.
3. Activa la casilla **HTTPS / SSL (Let's Encrypt)**. Traefik generará y renovará automáticamente tu certificado SSL gratuito.

### Paso 6: Desplegar

1. Haz clic en **Deploy**.
2. Dokploy compilará la imagen en 30-45 segundos.
3. Podrás verificar que todo está listo visitando:
   * `https://quiniela.tudominio.com/healthz` $\rightarrow$ Deberá responder `{"database":"connected","season":2026,"status":"ok"}`.
   * `https://quiniela.tudominio.com/picks` $\rightarrow$ Interfaz en vivo con actualizaciones SSE en tiempo real.

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
