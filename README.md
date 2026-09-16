# Banking System

Sistema de banca en línea con doble base de datos (PostgreSQL + TigerBeetle), backend en Go, frontend en React, y un asistente de IA que ejecuta operaciones financieras en lenguaje natural.

> **Sobre el asistente de IA:** el sistema implementa *tool calling* estilo OpenAI sobre OpenRouter. Los tools expuestos son funcionalmente equivalentes a los que expondría un servidor MCP — la diferencia es que OpenRouter no habla MCP directamente, expone una API compatible con OpenAI. Ver [la sección de decisiones técnicas](#decisiones-técnicas) para el detalle.

---

## Stack

| Capa | Tecnología | Versión |
|---|---|---|
| Backend | Go + Chi router | 1.24 + v5.3.2 |
| Base de datos de identidad | PostgreSQL | 16-alpine |
| Base de datos financiera | TigerBeetle | 0.17.9 |
| Frontend | React + Vite + Tailwind | 19 + 8 + 3.4 |
| Autenticación | JWT HS256 | golang-jwt/v5 |
| Hash de contraseñas | bcrypt | cost 12 |
| IA / Chat | OpenRouter | `openrouter/free` |
| Infraestructura | Docker + Docker Compose | v5 |

---

## Arquitectura

La decisión más importante del sistema es usar **dos bases de datos con responsabilidades distintas**.

```
┌─────────────────────────────────────────────────────────────┐
│                    FRONTEND (React + Vite)                   │
│                    Puerto 80 (nginx)                          │
└──────────────────────────┬──────────────────────────────────┘
                           │ HTTP + JWT
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                    BACKEND (Go 1.24 + Chi)                   │
│                    Puerto 8080                                │
│                                                              │
│  Auth · Cuentas · Transacciones · Chat IA · Lookup          │
└──────┬────────────────────┬──────────────────────┬──────────┘
       │                    │                      │
       ▼                    ▼                      ▼
┌─────────────┐   ┌──────────────────┐   ┌──────────────────┐
│ PostgreSQL  │   │   TigerBeetle    │   │   OpenRouter     │
│ 16          │   │   0.17.9         │   │   (LLM)          │
│             │   │                  │   │                  │
│ Usuarios    │   │ Cuentas          │   │ Tool calling     │
│ Auth        │   │ Balances         │   │                  │
│ Índice tx   │   │ Transferencias   │   │                  │
└─────────────┘   └──────────────────┘   └──────────────────┘
```

**PostgreSQL** guarda todo lo relacional: usuarios, contraseñas, alias, el índice de transacciones. **TigerBeetle** es la fuente de verdad financiera: cuentas, balances, transferencias.

La razón de separarlos es simple: guardar el saldo como un campo `INT` mutable en PostgreSQL es peligroso. Dos transacciones concurrentes pueden pisarse sin que te des cuenta. TigerBeetle usa contabilidad de doble entrada y garantiza que el dinero nunca se duplica ni se pierde, independientemente de la concurrencia.

**Regla de oro:** si tiene que ver con identidad → PostgreSQL. Si tiene que ver con dinero → TigerBeetle.

### Cómo funciona el modelo contable

Cada operación en TigerBeetle es una transferencia con dos lados:

```
Transfer {
  debit_account_id:  ← de dónde sale el dinero
  credit_account_id: ← a dónde entra el dinero
  amount:            en centavos
  code:              1=deposit, 2=withdrawal, 3=transfer
}
```

| Operación | Debit | Credit |
|---|---|---|
| Depositar | Banco (ID=1) | Usuario |
| Retirar | Usuario | Banco (ID=1) |
| Transferir | Usuario origen | Usuario destino |

El saldo de un usuario es simplemente `credits_posted − debits_posted`. Nunca se guarda como número — se calcula. Esto hace imposible que se corrompa.

Las cuentas de usuario tienen el flag `DebitsMustNotExceedCredits`, así que TigerBeetle rechaza directamente cualquier operación que dejaría el saldo negativo, sin que el backend necesite verificarlo primero.

---

## Cómo levantar el sistema

### Requisitos

- Docker y Docker Compose instalados
- API key de OpenRouter (gratuita en https://openrouter.ai)
- Puertos `80`, `8080`, `5432`, `3000` libres

### Pasos

```bash
# Clonar
git clone https://github.com/geanmunoz/banking-system
cd banking-system

# Configurar variables de entorno
cp .env.example .env
# Editar .env y agregar OPENROUTER_API_KEY

# Levantar todo
docker compose up -d --build
sleep 40

# Verificar
docker ps --filter name=banking-
curl -s http://localhost:8080/health | python3 -m json.tool
```

Si todo está bien, deberías ver esto:

```
banking-frontend     Up X seconds    0.0.0.0:80->80/tcp
banking-backend      Up X seconds    0.0.0.0:8080->8080/tcp
banking-postgres     Up X seconds (healthy)
banking-tigerbeetle  Up X seconds (healthy)
```

```json
{
  "postgres": "ok",
  "status": "healthy",
  "tigerbeetle": "ok"
}
```

### Acceso

- Frontend: http://localhost
- API: http://localhost:8080
- PostgreSQL: localhost:5432
- TigerBeetle: localhost:3000

```bash
# Bajar (mantiene datos)
docker compose down

# Bajar y resetear todo
docker compose down -v
```

---

## Credenciales de prueba

Los usuarios demo se crean automáticamente cuando el backend arranca en `APP_ENV=development`. No hay que hacer nada manual.

| Email | Password | Alias |
|---|---|---|
| `demo@banco.com` | `Demo1234!` | `demo` |
| `demo2@banco.com` | `Demo1234!` | `demo2` |

En la página de login hay un botón **"Usar credenciales demo →"** que autocompleta el formulario. `demo2` existe específicamente para probar transferencias entre usuarios.

En desarrollo, Compose también importa los primeros `TEST_DATA_LIMIT` usuarios de `datos-prueba.json` usando el flujo normal de registro y carga hasta `TEST_TRANSACTION_LIMIT` transacciones compatibles. El adaptador genera el hash, alias y cuenta TigerBeetle con el formato de este proyecto, filtra transacciones cuyos usuarios no fueron cargados y convierte los montos a centavos. Para cambiar las cantidades, edita `TEST_DATA_LIMIT` y `TEST_TRANSACTION_LIMIT` en `.env` y recrea los volúmenes.

---

## Variables de entorno

Todas están documentadas en `.env.example`. Las críticas:

### PostgreSQL

| Variable | Default | Descripción |
|---|---|---|
| `POSTGRES_HOST` | `postgres` | Hostname del servicio |
| `POSTGRES_PORT` | `5432` | Puerto |
| `POSTGRES_DB` | `banking` | Nombre de la base |
| `POSTGRES_USER` | `banking_user` | Usuario |
| `POSTGRES_PASSWORD` | `secret123` | Contraseña |

### TigerBeetle

| Variable | Default | Descripción |
|---|---|---|
| `TB_ADDRESS` | `172.30.0.10:3000` | IP fija del cluster (el cliente Go no acepta hostnames) |
| `TB_CLUSTER_ID` | `0` | Cluster ID |

### JWT

| Variable | Default | Descripción |
|---|---|---|
| `JWT_SECRET` | — | **Requerido.** Mínimo 32 chars |
| `JWT_EXPIRY_HOURS` | `24` | Vida del token |

### OpenRouter

| Variable | Default | Descripción |
|---|---|---|
| `OPENROUTER_API_KEY` | — | **Requerido.** Obtener en https://openrouter.ai/keys |
| `OPENROUTER_MODEL` | `openrouter/free` | Meta-router gratuito |

### App

| Variable | Default | Descripción |
|---|---|---|
| `APP_PORT` | `8080` | Puerto del backend |
| `APP_ENV` | `development` | Si es `development`, crea los usuarios demo |
| `RECONCILE_INTERVAL_MINUTES` | `5` | Intervalo del reconciliador |

```bash
# Generar JWT_SECRET seguro
openssl rand -hex 32
```

Usar **hex**, no base64. Docker Compose falla con `+`, `/` y `=` en las variables.

---

## API endpoints

15 endpoints REST en total.

### Autenticación

| Método | Endpoint | Auth | Descripción |
|---|---|---|---|
| POST | `/api/auth/register` | No | Crear usuario |
| POST | `/api/auth/login` | No | Login → JWT |
| POST | `/api/auth/logout` | No | Logout (stateless) |
| GET | `/api/auth/me` | JWT | Datos del usuario actual |

### Cuentas

| Método | Endpoint | Auth | Descripción |
|---|---|---|---|
| GET | `/api/account` | JWT | Info completa de la cuenta |
| GET | `/api/account/balance` | JWT | Saldo en centavos |

### Transacciones

| Método | Endpoint | Auth | Descripción |
|---|---|---|---|
| POST | `/api/transactions/deposit` | JWT | Depositar |
| POST | `/api/transactions/withdraw` | JWT | Retirar |
| POST | `/api/transactions/transfer` | JWT | Transferir |
| GET | `/api/transactions/history` | JWT | Historial paginado |
| POST | `/api/transactions/demo-topup` | JWT | Cargar $1000 de demo |

### Chat IA

| Método | Endpoint | Auth | Descripción |
|---|---|---|---|
| POST | `/api/chat` | JWT | Mensaje al asistente |
| POST | `/api/chat/confirm` | JWT | Confirmar operación pendiente |

### Utilidades

| Método | Endpoint | Auth | Descripción |
|---|---|---|---|
| GET | `/api/users/lookup` | JWT | Buscar usuario por email o alias |
| GET | `/health` | No | Estado del sistema |

Documentación completa de cada endpoint en `docs/api-reference.md`.

---

## Funcionalidades

### Autenticación

Login y registro estándar con JWT HS256 (24h). Hay dos detalles de seguridad que vale la pena mencionar: los errores de "email no existe" y "password incorrecta" devuelven el mismo código (`INVALID_CREDENTIALS`) para no filtrar qué cuentas existen, y cuando el email no existe se ejecuta igual un dummy bcrypt para mantener el tiempo de respuesta constante y hacer inútil el timing attack.

### Cuentas

Al registrarse, el backend crea automáticamente la cuenta en TigerBeetle. El alias se genera del email (`juan@example.com` → `@juan`, con sufijo si está tomado). El saldo es en tiempo real desde TigerBeetle, nunca de un cache.

### Transacciones

Depósito, retiro y transferencia con validación completa. El retiro valida el saldo dos veces: primero en Go (para dar un error temprano), después en TigerBeetle (la garantía real contra condiciones de carrera). Todas las operaciones de escritura aceptan un header `Idempotency-Key` — si el cliente reintenta por timeout, no se duplica el movimiento.

### Chat con IA

El asistente entiende comandos en español:

- *"¿Cuánto tengo?"* → consulta el saldo
- *"Muéstrame mis últimas 5 transacciones"* → historial
- *"Deposita $50"* → pide confirmación antes de ejecutar
- *"Transfiere $10 a demo2@banco.com"* → pide confirmación

El flujo de confirmación funciona así:

```
1. Usuario: "Deposita $50"
2. Backend: crea token, responde "¿Confirmas?"
3. Usuario: hace clic en "Confirmar"
4. Backend: valida token, ejecuta la operación
```

El backend no confía en el modelo para ejecutar. El modelo propone, el usuario confirma, el backend ejecuta. El token es de un solo uso, expira en 5 minutos y está atado al usuario que lo creó.

---

## Testing

| Tipo | Cantidad |
|---|---|
| Unitarios | 72 |
| Integración | 53 |
| **Total** | **125** |

```bash
cd backend

# Unitarios (sin dependencias externas)
go test ./...

# Integración (requiere contenedores corriendo)
TB_TEST_ADDRESS=172.30.0.10:3000 \
POSTGRES_TEST_DSN="host=172.30.0.2 port=5432 user=banking_user password=secret123 dbname=banking sslmode=disable" \
go test -tags=integration ./...
```

### Flujo de prueba manual recomendado

1. Abrir http://localhost
2. Login con `demo@banco.com` / `Demo1234!`
3. Preguntar al chat: *"¿Cuánto tengo?"*
4. Cargar $1,000 de demo
5. Por chat: *"Deposita $50"* → Confirmar
6. Por chat: *"Transfiere $10 a demo2@banco.com"* → Confirmar
7. Ir a Transacciones y hacer un retiro manual
8. Ver el historial
9. Cerrar sesión y volver a entrar

---

## Decisiones técnicas

### Doble base de datos

Ya lo expliqué en la sección de arquitectura, pero lo resumo: PostgreSQL para identidad, TigerBeetle para dinero. Guardar el saldo como campo mutable en PostgreSQL crea riesgos de concurrencia que TigerBeetle resuelve por diseño.

### Estado PENDING/ACTIVE en el registro

PostgreSQL y TigerBeetle no comparten transacción ACID, así que hay un riesgo: si el INSERT en PG funciona pero la creación de la cuenta en TigerBeetle falla, el usuario queda en un estado inconsistente.

La solución es insertar en PG con `status=PENDING`, crear la cuenta en TigerBeetle, y después marcar como `ACTIVE`. Si algo falla en el medio, el usuario queda `PENDING` y el reconciliador lo recupera en los próximos 5 minutos.

### Reconciliador

Un goroutine que corre cada 5 minutos, busca usuarios `PENDING`, verifica si su cuenta en TigerBeetle existe, la crea si no existe, y los marca como `ACTIVE`. Es idempotente — puede correr N veces sin efectos secundarios.

### Idempotencia

El header `Idempotency-Key` en operaciones de escritura deriva el ID de la transferencia en TigerBeetle con:

```
TBTransferID = SHA256(userID + 0x00 + idempotencyKey)[:16]
```

Mismo request → mismo ID → TigerBeetle devuelve `TransferExists` → se trata como éxito. Esto hace que los reintentos por timeout sean seguros.

### Confirmación con token de un solo uso

Confiar en el modelo para ejecutar operaciones financieras es riesgoso. Un jailbreak, un cambio de proveedor, o un modelo diferente no deben poder ejecutar transferencias sin el usuario.

La solución: el modelo propone la operación (tool call), el backend crea un `PendingOperation` con un token UUID, el usuario confirma con ese token, y recién ahí el backend ejecuta. El token tiene un solo uso, expira en 5 minutos, y está atado al usuario correcto. La validación + consumo es atómica (una sola query `UPDATE ... WHERE`).

**Nota sobre la nomenclatura:** esto se llama "confirmación en dos fases" porque la operación tiene dos etapas — propuesta y ejecución. No es 2PC (two-phase commit), que es un protocolo de bases de datos distribuidas. Es un patrón de aplicación con token de un solo uso, adecuado para la interacción usuario-LLM.

### Tool calling vs MCP

La prueba pedía integración de IA con MCP. No implementé MCP literal por una razón concreta: OpenRouter no habla MCP, expone una API compatible con OpenAI (chat completions + `tools`). MCP es un protocolo entre un host como Claude Desktop y un servidor de tools — OpenRouter no actúa como host MCP.

Lo que sí implementé son los tools equivalentes: `get_balance`, `get_history`, `deposit`, `withdraw`, `transfer`. La interfaz que el usuario experimenta es la misma. El modelo puede usar esos tools y el backend controla exactamente qué puede hacer con ellos.

### Anti-enumeración

Si el error de "email no existe" fuera distinto al de "password incorrecta", un atacante podría enumerar qué usuarios están registrados simplemente probando emails. Ambos casos devuelven `INVALID_CREDENTIALS`. Además, cuando el email no existe, el backend ejecuta igual un dummy bcrypt para que el timing de la respuesta sea idéntico al caso en que sí existe.

---

## Estructura del repositorio

```
banking-system/
├── .env                      # Secretos (gitignored)
├── .env.example              # Template
├── docker-compose.yml        # 4 servicios
├── README.md
│
├── db/
│   └── init.sql              # Schema PostgreSQL
│
├── scripts/
│   └── tb-entrypoint.sh      # Entrypoint TigerBeetle
│
├── docs/                     # Documentación técnica
│   ├── api-reference.md      # Referencia completa de la API
│   ├── architecture.md
│   └── ...
│
├── backend/
│   ├── Dockerfile
│   ├── go.mod, go.sum
│   ├── cmd/server/main.go
│   └── internal/
│       ├── apperr/           # Errores unificados
│       ├── models/           # Structs compartidos
│       ├── db/               # PostgreSQL
│       ├── tigerbeetle/      # TigerBeetle
│       ├── password/         # bcrypt
│       ├── auth/             # Auth + JWT
│       ├── recovery/         # Reconciliador
│       ├── transactions/     # Deposit, Withdraw, Transfer
│       ├── account/          # Info, Balance
│       └── chat/             # Chat IA + tools
│
└── frontend/
    ├── Dockerfile
    ├── nginx.conf
    ├── package.json
    ├── vite.config.js
    └── src/
        ├── api/
        ├── contexts/
        ├── hooks/
        ├── components/
        └── pages/
```

---

## Problemas conocidos

**Rate limit de OpenRouter:** el meta-router `openrouter/free` selecciona un modelo gratuito disponible. Bajo alta demanda puede devolver 429. La aplicación reintenta con backoff exponencial, pero si el problema persiste, se puede configurar un modelo específico con `OPENROUTER_MODEL`.

**TigerBeetle requiere IP fija:** el cliente Go v0.17.9 no acepta hostnames en la dirección del cluster. Por eso `TB_ADDRESS` usa `172.30.0.10:3000` con una red Docker explícita (`ipv4_address`). Si cambiás la subred de Docker, hay que actualizar esa variable.

**Logout stateless:** el backend no mantiene blacklist de JWT. El token sigue siendo válido hasta que expira. El cierre de sesión es responsabilidad del cliente (borrar el token de `localStorage`). Esto es deuda técnica documentada.

---

## Autor

**Gean Munoz** — [@geanmunoz](https://github.com/geanmunoz)