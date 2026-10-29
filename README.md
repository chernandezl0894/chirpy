<div align="center">
  <img src="assets/logo.png" alt="Chirpy logo" width="120"/>
  <h1>Chirpy</h1>
  <p>Una API REST estilo mini-Twitter construida en Go, inspirada en el curso <em>Boot.dev Backend Developer</em>.</p>
</div>

---

Chirpy es un clon simplificado de Twitter: los usuarios se registran, inician sesión, publican "chirps" de máximo 140 caracteres y pueden ser ascendidos a la categoría **Chirpy Red** mediante webhooks de un servicio de pagos ficticio llamado Polka.

## Stack tecnológico

| Capa | Tecnología |
|---|---|
| Lenguaje | Go 1.22 |
| HTTP | `net/http` (stdlib) con `http.ServeMux` y pattern routing (`GET /api/chirps/{id}`) |
| Base de datos | PostgreSQL |
| Driver | [`lib/pq`](https://github.com/lib/pq) |
| Migrations | [`goose`](https://github.com/pressly/goose) |
| SQL type-safe codegen | [`sqlc`](https://docs.sqlc.dev) — genera structs y funciones Go desde queries SQL |
| Tokens de acceso | JWT (HS256) con [`golang-jwt/jwt/v5`](https://github.com/golang-jwt/jwt) |
| Hashing de contraseñas | Argon2id ([`alexedwards/argon2id`](https://github.com/alexedwards/argon2id)) |
| Refresh tokens | Tokens aleatorios opacos (256 bits, hex) almacenados en la BD |
| Configuración | Variables de entorno con [`joho/godotenv`](https://github.com/joho/godotenv) |

## Estructura del proyecto

```
chirpy/
├── main.go                 # Handlers HTTP, enrutado y arranque del servidor
├── internal/
│   ├── auth/               # JWT, Argon2id, refresh tokens, helpers de headers
│   │   └── auth_test.go    # Tests unitarios del paquete de auth
│   └── database/           # Código generado por sqlc (no editar a mano)
├── sql/
│   ├── schema/             # Migrations numeradas (001_users.sql, 002_chirps.sql, ...)
│   └── queries/            # Queries SQL con anotaciones -- name: <Nombre> :one|:many|:exec
├── sqlc.yaml               # Configuración de sqlc
├── index.html              # Página servida en /app/
└── assets/                 # Logo y estáticos
```

> [!NOTE]
> Arquitectura intencionalmente simple: handlers en `main.go`, acceso a datos con funciones generadas por sqlc en `internal/database`, y utilidades de seguridad puras en `internal/auth`. Cada capa puede separarse más (p. ej. mover handlers a `internal/handler`) a medida que la app crece.

## Endpoints

| Método | Ruta | Auth | Descripción |
|---|---|---|---|
| `GET` | `/api/healthz` | — | Estado del servicio |
| `GET` | `/app/` | — | Archivos estáticos + contador de visitas |
| `GET` | `/admin/metrics` | — | HTML con el número de visitas |
| `POST` | `/admin/reset` | `PLATFORM=dev` | Borra todos los usuarios (solo desarrollo) |
| `POST` | `/api/users` | — | Crea usuario `{email, password}` |
| `PUT` | `/api/users` | Bearer JWT | Actualiza el usuario autenticado |
| `POST` | `/api/login` | — | `{email, password, expires_in_seconds?}` → usuario + `token` + `refresh_token` |
| `POST` | `/api/refresh` | Bearer refresh | Emite un nuevo JWT si el refresh token es válido |
| `POST` | `/api/revoke` | Bearer refresh | Revoca el refresh token enviado |
| `GET` | `/api/chirps` | — | Lista chirps. Query params opcionales: `author_id` (UUID, filtra en la BD) y `sort` (`asc` \| `desc`, por `created_at`, default `asc`) |
| `GET` | `/api/chirps/{id}` | — | Un chirp por ID |
| `POST` | `/api/chirps` | Bearer JWT | Crea chirp (≤140 caracteres, malas palabras reemplazadas por `****`) |
| `DELETE` | `/api/chirps/{id}` | Bearer JWT | Solo el autor puede borrar su chirp |
| `POST` | `/api/polka/webhooks` | API key | Evento `user.upgraded` → marca al usuario como Chirpy Red |

## Cómo usarlo

### 1. Requisitos

- Go ≥ 1.22
- PostgreSQL en ejecución
- Opcional (para regenerar código): `goose` y `sqlc`

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

### 2. Base de datos

```bash
sudo apt install postgresql postgresql-contrib
sudo -u postgres psql
```

```sql
CREATE DATABASE chirpy;
\c chirpy
ALTER USER postgres WITH PASSWORD 'postgres';
```

### 3. Variables de entorno

Crea un archivo `.env` en la raíz:

```bash
DB_URL="postgres://postgres:postgres@127.0.0.1:5432/chirpy?sslmode=disable"
PLATFORM="dev"
TOKEN_STRING="secret"
POLKA_KEY="cambia-esta-clave-por-una-generada-con-openssl-rand-hex-32"
```

> [!TIP]
> Para un `TOKEN_STRING` fuerte: `openssl rand -base64 64`.

### 4. Migraciones

Desde `sql/schema/`:

```bash
goose postgres "postgres://postgres:postgres@127.0.0.1:5432/chirpy" up
```

### 5. Generar el código de acceso a datos

Desde la raíz del proyecto, después de editar `sql/queries/*.sql`:

```bash
sqlc generate
```

### 6. Compilar y ejecutar

```bash
go build && ./chirpy
# o: go run .
curl http://localhost:8080/api/healthz
```

### 7. Probar la API

```bash
# Registrar usuario
curl -s -X POST http://localhost:8080/api/users \
  -H "Content-Type: application/json" \
  -d '{"email":"me@example.com","password":"***"}'

# Chirps: todos, por autor, ordenados
curl -s "http://localhost:8080/api/chirps"
curl -s "http://localhost:8080/api/chirps?author_id=<uuid>&sort=desc"

# Crear chirp autenticado
curl -s -X POST http://localhost:8080/api/chirps \
  -H "Authorization: Bearer <jwt>" \
  -H "Content-Type: application/json" \
  -d '{"body":"hello chirpy!"}'
```

### 8. Tests

```bash
go test ./...
```

---

## Guía paso a paso para replicar Chirpy en otro lenguaje o framework

Esta es la ruta de aprendizaje completa. Cada paso funciona igual en Node/Express, Python/FastAPI, Ruby/Rails, Rust/Axum, C#/ASP.NET o Java/Spring; solo cambian las librerías.

### Paso 1 — Levanta tu primer servidor HTTP

**Objetivo:** responder `200 OK` en `GET /api/healthz`.

1. Crea el proyecto mínimo (en Go: `go mod init <ruta>` y un `main.go`).
2. Arranca un servidor que escuche en un puerto (`http.Server` en Go, `express()` en Node, `uvicorn` en FastAPI, `Kestrel` en ASP.NET).
3. Maneja `SIGINT`/`SIGTERM` para apagar con gracia (`server.Close()` / `process.on('SIGINT')`).
4. Mueve el puerto y las credenciales a variables de entorno (`dotenv` en cualquier ecosistema).

> [!TIP]
> Antes de escribir handlers, verifica que `curl http://localhost:8080/api/healthz` devuelve algo. Todo lo demás se construye sobre esto.

### Paso 2 — Request, response y parsing de JSON

**Objetivo:** dos endpoints de texto plano y uno que acepte JSON.

1. Responde texto con `Content-Type: text/plain` y un status explícito (`200`, `400`, `500`).
2. Crea un endpoint `POST` que reciba JSON en el body, lo deserealice al tipo/struct de tu lenguaje y lo re-emita como JSON (`201 Created`).
3. Valida la entrada: si el JSON es inválido → `400`; si falta un campo obligatorio → `400`. Nunca asumas que el cliente es honesto.
4. Define tus tipos de dominio (usuario, chirp) con nombres en `snake_case` en el JSON y en `PascalCase`/`CamelCase` en el código.

### Paso 3 — Routing

**Objetivo:** URLs limpias y parámetros de ruta.

1. Agrupa rutas por recurso: `/api/users`, `/api/chirps`.
2. Usa path parameters: `GET /api/chirps/{id}` — en tu framework busca el equivalente (Go 1.22 `ServeMux`, Express `:id`, FastAPI `{id}` en la decoración de ruta).
3. Resuelve colisiones de rutas (orden/expresividad del router) y usa siempre métodos HTTP semánticos: `GET` lee, `POST` crea, `PUT` reemplaza, `DELETE` elimina.
4. Añade un handler de estáticos servido desde disco bajo un prefijo (`/app/`).

### Paso 4 — Arquitectura de la API y query parameters

**Objetivo:** separar responsabilidades y añadir filtros/orden.

1. Divide en capas: **transporte** (handlers) → **dominio/reglas** (limpiar malas palabras, validar longitud) → **persistencia** (funciones de BD). En Chirpy, `internal/auth` y `internal/database` son las capas inferiores.
2. Crea un objeto de configuración inyectable (en Go, un struct `apiConfig` con métodos-handler elimina variables globales y facilita tests).
3. Soporta query parameters opcionales: `?author_id=<id>` filtra, `?sort=asc|desc` ordena. Parámetro ausente → comportamiento por defecto documentado.
4. Regla de rendimiento: **filtra en la base de datos** (`WHERE user_id = ?`) y deja en memoria solo operaciones baratas como reordenar una página pequeña.
5. Estandariza las respuestas de error: `{"error": "..."}` con el status HTTP correcto. Nunca detalles internos en el mensaje al cliente.

### Paso 5 — Storage: base de datos relacional y migrations

**Objetivo:** persistencia real con PostgreSQL (o el SQL de tu preferencia).

1. Modela el esquema con relaciones: `users (1) → (N) chirps`, clave foránea con `ON DELETE CASCADE`.
2. Usa **migrations numeradas y reversibles** (`goose`, `alembic`, `knex`, `ActiveRecord`, `diesel`). Nunca edites una migration ya aplicada: crea la siguiente.
3. Elige un patrón de acceso a SQL. Opciones: ORM completo, query-builder, o **codegen a partir de SQL escrito por ti** (sqlc en Go). El último te da tipos seguros sin perder SQL.
4. Usa `UUID` para las IDs (generadas en la BD, p. ej. `gen_random_uuid()`) para que no se puedan adivinar.
5. Almacena timestamps en UTC y ponles índices si filtras/ordenas por ellos.

### Paso 6 — Autenticación con JWT y refresh tokens

**Objetivo:** identificar al usuario en cada request sin sesión en servidor.

1. **Contraseñas:** nunca en texto plano. Hash con Argon2id (recomendado) o bcrypt; compara con la función `verify`/`check` de tu librería.
2. **Access token (JWT):** al hacer login, firma un token que contenga el `user_id` y una expiración corta (≤1 h). En Chirpy se firma HS256 con `TOKEN_STRING`.
3. **Extracción del token:** convención `Authorization: Bearer <token>`. Escribe un helper que lo lea del header y devuelva error si falta.
4. **Middleware/guard:** en endpoints protegidos, valida firma + expiración; si falla → `401 Unauthorized`.
5. **Refresh tokens:** como los JWT no se pueden "revocar" barato, emite además un token opaco aleatorio (256 bits), guárdalo en la BD con `expires_at` (60 días) y `revoked_at` (nullable). `POST /api/refresh` lo valida y emite un JWT nuevo; `POST /api/revoke` lo marca como revocado.
6. Limita la expiración pedida en login a un máximo (`expires_in_seconds` nunca supera 1 h).

> [!WARNING]
> No uses un secreto débil ni lo subas al repo en producción. Un JWT con clave comprometida permite suplantar a cualquier usuario.

### Paso 7 — Autorización

**Objetivo:** además de *quién eres*, decidir *qué puedes hacer*.

1. Distingue `401` (no autenticado) de `403` (autenticado pero sin permiso).
2. Regla de propiedad: para `DELETE /api/chirps/{id}`, carga el recurso y compara su `user_id` con el del token → `403` si no coincide.
3. Recursos inexistentes → `404`.
4. Restringe endpoints de administración por entorno (`POST /admin/reset` solo si `PLATFORM=dev`) → `403` en producción.
5. Roles/flags de usuario: la marca `is_chirpy_red` muestra cómo añadir privilegios de forma incremental.

### Paso 8 — Webhooks

**Objetivo:** que un servicio externo (Polka) notifique a tu API.

1. Expón `POST /api/polka/webhooks` con un payload fijo: `{"event": "user.upgraded", "data": {"user_id": "..."}}`.
2. **Autentica al emisor** con una API key secreta en un header (`x-polka-key`). Si no coincide → `401`.
3. **Idempotencia y ruido:** ignora eventos desconocidos con `204 No Content` (no `4xx`: un webhook desconocido no debe reintentarse eternamente).
4. Usuario inexistente → `404`; actualiza el estado y responde `204`.
5. Valida primero el header y después el body: no desperdicies parsing en requests no autenticadas.

### Paso 9 — Tests

**Objetivo:** confianza para refactorizar.

1. **Unit tests de la lógica pura** (`internal/auth` es el ejemplo perfecto): hacer un JWT, validarlo con la clave correcta y con una incorrecta, expiración, hash/verify de contraseñas, parsing de headers. Usa tablas de casos.
2. **Tests de handlers** con `httptest` (o `supertest`/`httpx`/`WebApplicationFactory`): construye el `apiConfig`, llama al handler y aserta status y body. Para BD, usa una base de test o mocks del repositorio.
3. **Postman/Newman:** una colección con requests encadenados (login → token → crear chirp) para pruebas end-to-end contra un entorno `dev` real.
4. Meta-métrica: `go test ./...` (o equivalente) debe pasar verde antes de cada push; integra en CI.

### Paso 10 — Documentación

**Objetivo:** que otra persona (o tú en 6 meses) pueda usar y extender la API.

1. **README:** qué es, stack, estructura, setup paso a paso, cómo correrlo y ejemplos `curl` por endpoint.
2. **Referencia de API:** tablas de endpoints con método, auth requerida, body/params, y códigos de respuesta (`200/201/204/400/401/403/404/500`).
3. **OpenAPI/Swagger** como siguiente nivel: describe la API en YAML y genera docs interactivas o clientes (`go-openapi`, `FastAPI` la autogenera, `Swashbuckle` en .NET).
4. Documenta las **decisiones**, no solo el uso: por qué Argon2id, por qué filtrar en BD, por qué refresh tokens opacos. Un `docs/` o comentarios en el código sirven.
5. Mantén la doc en el repo, versionada con el código.

> [!TIP]
> El orden de los pasos 1→8 es exactamente la progresión del curso de Boot.dev: cada capítulo añade una pieza (JSON → routing → queries → BD → JWT → refresh → authz → webhooks). Si replicás Chirpy en otro stack, seguila en el mismo orden para validar cada capa antes de pasar a la siguiente.

## Licencia

MIT — ver `LICENSE`.
