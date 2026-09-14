# Arquitectura — Sistema de Banca en Línea

## Decisiones de diseño

### D1 — Identificadores duales

Cada usuario tiene dos IDs:

| ID | Dónde vive | Tipo | Propósito |
|---|---|---|---|
| `user_id` | PostgreSQL | `UUID` | Auth, JWT, relaciones |
| `tb_account_id` | PostgreSQL (columna) + TigerBeetle | `uint128` (BYTEA en PG) | Operaciones financieras |

**Regla:** el `tb_account_id` se genera **una sola vez, en el registro**, como `uint128` **aleatorio criptográficamente seguro** (no contador, no derivado del user_id). Se guarda en PostgreSQL como `BYTEA` (16 bytes) y en TigerBeetle como `Uint128`.

**Por qué no derivarlo del user_id:** un hash determinístico puede colisionar. Un contador secuencial es predecible. Un aleatorio de 128 bits hace imposible que un atacante adivine el ID de otro usuario.

---

### D2 — Sesión y logout

JWT stateless, expiración 24h. **Sin blacklist.**

- `POST /api/auth/logout` devuelve 200 y no hace nada en el backend.
- El frontend borra el token de `localStorage` al recibir la respuesta.
- No hay refresh token (fuera de alcance para la prueba).

**Trabajo futuro documentado en README:** refresh tokens con rotación + blacklist en Redis.

---

### D3 — Manejo de errores

Tipo único `AppError` en `internal/apperr/`. Los services devuelven `*AppError`. Los handlers lo mapean a HTTP.

```go
type AppError struct {
    Code       string // código interno: "INVALID_EMAIL", "INSUFFICIENT_FUNDS"
    Message    string // mensaje para el cliente (en español)
    HTTPStatus int    // 400, 401, 403, 404, 500
    Err        error  // error original (no se expone al cliente, se loggea)
}
```

Constructores: `apperr.BadRequest`, `apperr.Unauthorized`, `apperr.Forbidden`, `apperr.NotFound`, `apperr.Conflict`, `apperr.Internal`.

**Regla:** ningún handler devuelve `http.Error` directo. Todo pasa por `apperr.Write(w, err)`.

---

### D4 — Validación

Struct tags en los DTOs de request (`internal/*/handler.go`), validadas con `go-playground/validator/v10` **dentro del service**, no en el handler.

Flujo:
1. Handler deserializa JSON → struct DTO.
2. Handler llama `service.Register(ctx, dto)`.
3. Service valida con `validator.Struct(dto)`. Si falla → `apperr.BadRequest`.
4. Service ejecuta lógica.

**Regla:** el handler no valida campos individualmente. Solo deserializa y delega.

---

### D5 — Paso de datos entre capas

Structs compartidos en `internal/models/`. Handlers definen DTOs HTTP (con tags `json` y `validate`) que viven en el mismo archivo del handler.

| Capa | Tipo de dato |
|---|---|
| Handler (HTTP) | DTO con tags `json` + `validate` |
| Service | `models.*` + DTOs como input |
| DB | `models.*` |

**Regla:** los DTOs HTTP nunca salen del paquete handler. Los `models.*` son la representación interna compartida.

---

## Estructura de carpetas

```
backend/
├── cmd/server/main.go
├── internal/
│   ├── apperr/apperr.go
│   ├── models/models.go
│   ├── auth/
│   │   ├── handler.go
│   │   ├── service.go
│   │   └── middleware.go
│   ├── accounts/
│   │   ├── handler.go
│   │   └── service.go
│   ├── transactions/
│   │   ├── handler.go
│   │   └── service.go
│   ├── chat/                    
│   │   ├── handler.go
│   │   ├── service.go
│   │   └── tools.go
│   └── db/
│       ├── postgres.go
│       └── tigerbeetle.go
├── Dockerfile
└── go.mod
```

**Nota sobre `internal/chat/`:** implementamos **tool calling estilo OpenAI** sobre OpenRouter, no MCP literal. El nombre `chat/`. El README explica "tool calling (MCP-style)".

---

## Correcciones aplicadas vs. plan original

| # | Error original | Corrección |
|---|---|---|
| 1 | "MCP Server → OpenRouter" | Tool calling estilo OpenAI. Paquete `internal/chat/`. |
| 2 | `debits_must_not_exceed_credits = false` | Cuenta usuario: flag `true`. Banco (ID=1): flag `0`. |
| 3 | Docker Compose levanta TB directo | Entrypoint idempotente con `format` condicional + `--development`. |
| 4 | `tb_account_id BIGINT` | `BYTEA NOT NULL UNIQUE` (uint128 = 16 bytes). |
| 5 | Logout con blacklist | Stateless. Frontend borra token. |
| 6 | bcrypt cost 12 | Cost 10. |
| 7 | Bloque MCP 2-3h | 3-4h realista. |
| 8 | TB sin `security_opt` | `seccomp=unconfined` obligatorio en Docker 25+. |
| 9 | Go 1.26 en go.mod | Go 1.24. |

---

## Stack final

| Capa | Tecnología | Versión |
|---|---|---|
| Backend | Go + Chi | 1.24 + v5 |
| Auth DB | PostgreSQL | 16-alpine |
| Finance DB | TigerBeetle | 0.17.9 |
| Frontend | React + Vite + Tailwind | 18/19 + 5 + 3 |
| Auth | JWT (golang-jwt/v5) | v5 |
| IA | OpenRouter → Claude 3.5 Haiku | tool calling |
| Infra | Docker + Compose | v5 |