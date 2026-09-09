# 🏈 NFL Quiniela 2026

Plataforma web moderna, ligera y de alto rendimiento para Quinielas (Pick'em Pools) de la temporada 2026 de la NFL. Construida con **Go (Golang)**, **SQLite** (con ruta directa de migración a **PostgreSQL**), **HTMX**, **Alpine.js** y **Tailwind CSS**, con integración automática de marcadores en vivo desde la API pública de **ESPN** y controles de administración completos.

---

## ⚡ Características Principales

- **Arquitectura Ligera en Go**: Binario único ejecutable con plantillas HTML y recursos estáticos embebidos (`embed.FS`).
- **Base de Datos Multi-Dialecto**: SQLite integrado por defecto (sin dependencias CGO) y compatibilidad total con PostgreSQL con solo cambiar una variable de entorno (`DATABASE_URL`).
- **Frontend Reactivo con HTMX & Alpine.js**:
  - Guardado instantáneo de pronósticos sin recargar la página.
  - Actualización automática de la tabla de posiciones en tiempo real.
  - Conteo regresivo en vivo hasta la patada inicial de cada partido.
  - Revelación comunitaria de pronósticos una vez que el partido inicia.
- **Mecánicas de Juego Flexibles**:
  - **Bloqueo Individual por Partido**: Cada juego se congela automáticamente al llegar su hora de inicio (Kickoff).
  - **Modos de Puntuación Configurables desde el Panel de Admin**:
    1. *Puntaje Ponderado + Bonos Extra*: Ganador (10 pts) + Bono Marcador Exacto (+5 pts) + Bono Margen (+2 pts) + Criterio de desempate en MNF.
    2. *1 Punto por Ganador + Desempate Puro*: Ganador (1 pt) y el marcador solo se usa para desempatar la semana.
- **Sincronización Automática con ESPN**:
  - Descarga partidos, horarios, estados (programado, en juego, final) y marcadores en vivo.
  - Designación automática del juego de Lunes por la Noche (Monday Night Football) como partido de desempate.
  - Panel de anulación manual para administradores (editar marcadores, forzar bloqueos, crear partidos).
- **Sembrado Automático**:
  - Los 32 equipos de la NFL con logos oficiales de alta resolución, colores y divisiones.
  - Las 18 semanas de temporada regular y los Playoffs (Wild Card, Divisional, Conferencia y Super Bowl).

---

## 🚀 Inicio Rápido

### 1. Requisitos
- [Go 1.22+](https://golang.org/dl/)

### 2. Ejecutar la Aplicación
```bash
# Clonar o entrar al directorio del proyecto
cd nfl-quiniela-2026

# Ejecutar el servidor
go run main.go
```

Abre tu navegador en:
👉 **[http://localhost:8080](http://localhost:8080)**

---

## 🔑 Credenciales por Defecto (Administrador)

Al ejecutarse por primera vez, el sistema crea automáticamente una cuenta de administrador:

- **Usuario**: `admin`
- **Contraseña**: `admin123`
- **Correo**: `admin@quiniela.com`

---

## 🗄️ Migración a PostgreSQL

El proyecto utiliza sentencias SQL estándar y un repositorio agnóstico. Para migrar de SQLite a PostgreSQL en producción (Supabase, Neon, AWS RDS, Docker, etc.):

1. Configura las siguientes variables en tu archivo `.env`:
```env
DB_TYPE=postgres
DATABASE_URL=postgres://usuario:contraseña@localhost:5432/quiniela_db?sslmode=disable
```
2. Inicia la aplicación (`go run main.go`). El sistema ejecutará automáticamente las migraciones necesarias en PostgreSQL.

---

## 🧪 Pruebas Automatizadas

Para ejecutar todas las pruebas unitarias y de integración:

```bash
go test ./... -v
```

Las suites de pruebas cubren:
- Operaciones CRUD, transacciones y semillas de base de datos (`db/`)
- Mapeo y normalización de la API de ESPN (`services/espn/`)
- Modos de puntuación, bonos y resolución de desempates (`services/scoring/`)
- Registro, inicio de sesión, bloqueo de pronósticos y rutas de administrador (`handlers/`)

---

## 🛠️ Estructura del Proyecto

```
nfl-quiniela-2026/
├── config/              # Configuración y variables de entorno (.env)
├── db/                  # Esquema SQL, modelos y capa de persistencia
├── handlers/            # Manejadores HTTP, enrutador y motor de plantillas
├── services/
│   ├── auth/            # Hashing de contraseñas (bcrypt) y cookies de sesión
│   ├── espn/            # Cliente API de ESPN y sincronizador en segundo plano
│   └── scoring/         # Motor de cálculo de puntuaciones y tablas de posiciones
├── static/              # Estilos CSS y JavaScript para el cliente
├── templates/           # Plantillas HTML con HTMX y Alpine.js
│   ├── layouts/         # Diseño base (base.html con Tailwind)
│   ├── pages/           # Vistas completas (pronósticos, posiciones, admin, reglas)
│   └── partials/        # Componentes parciales para intercambios HTMX
├── .env.example         # Ejemplo de variables de entorno
└── main.go              # Punto de entrada y servidor HTTP con cierre ordenado
```

---

## 📜 Licencia
Proyecto desarrollado para la Quiniela NFL 2026.
