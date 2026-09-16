# 📘 Banking System — API Reference

**Versión:** 1.0
**Fecha:** 15 de septiembre de 2026
**Estado:** Backend completo
**Fuente de verdad:** Este documento

---

## Índice

1. [Convenciones generales](#1-convenciones-generales)
2. [Formato de errores](#2-formato-de-errores)
3. [Autenticación](#3-autenticación)
4. [Cuentas](#4-cuentas)
5. [Transacciones](#5-transacciones)
6. [Chat IA](#6-chat-ia)
7. [Utilidades](#7-utilidades)
8. [Tabla resumen](#8-tabla-resumen)
9. [Códigos de error](#9-códigos-de-error)
10. [Ejemplos end-to-end](#10-ejemplos-end-to-end)
11. [Notas para el frontend](#11-notas-para-el-frontend)

---

## 1. Convenciones generales

### 1.1 Base URL

```
http://localhost:8080
```

### 1.2 Formato de datos

- **Requests y responses:** JSON (`Content-Type: application/json`).
- **Montos:** siempre en **centavos** (`int64`). `$10.50 = 1050`.
- **IDs de cuenta TigerBeetle:** hex de 32 caracteres (uint128).
- **Fechas:** ISO 8601 UTC (`2026-09-15T18:30:00Z`).
- **UUIDs:** formato estándar (`550e8400-e29b-41d4-a716-446655440000`).

### 1.3 Autenticación

Los endpoints protegidos requieren el header:

```
Authorization: Bearer <jwt>
```

El JWT se obtiene de `POST /api/auth/login`. Expira en 24 horas.

### 1.4 Headers estándar

**Request (opcionales excepto Authorization):**

| Header | Requerido | Descripción |
|---|---|---|
| `Authorization` | Solo en endpoints protegidos | `Bearer <jwt>` |
| `Content-Type` | Sí (cuando hay body) | `application/json` |
| `Idempotency-Key` | No | UUID v4 para idempotencia (solo escrituras) |

**Response:**

| Header | Valor |
|---|---|
| `Content-Type` | `application/json` |

### 1.5 Códigos HTTP

| Código | Significado |
|---|---|
| 200 | OK |
| 201 | Created |
| 204 | No Content |
| 400 | Bad Request (validación) |
| 401 | Unauthorized (sin JWT o credenciales inválidas) |
| 403 | Forbidden (usuario no activo) |
| 404 | Not Found |
| 409 | Conflict (recurso duplicado) |
| 500 | Internal Server Error |
| 503 | Service Unavailable |

---

## 2. Formato de errores

**Todos los errores tienen la misma estructura:**

```json
{
  "code": "INVALID_CREDENTIALS",
  "message": "Credenciales inválidas"
}
```

- **`code`:** identificador técnico estable. El frontend puede usarlo para lógica.
- **`message`:** mensaje para mostrar al usuario. En español, listo para UI.

**Regla:** el `code` nunca cambia; el `message` puede mejorar sin romper clientes.

---

## 3. Autenticación

### 3.1 `POST /api/auth/register`

Registra un usuario nuevo.

**Auth:** No requerida.

**Headers:** `Content-Type: application/json`.

**Request body:**

```json
{
  "email": "juan@example.com",
  "password": "TestPassword123!",
  "full_name": "Juan Pérez"
}
```

| Campo | Tipo | Requerido | Validación |
|---|---|---|---|
| `email` | string | Sí | Formato email, ≤255 chars, único |
| `password` | string | Sí | 8-72 bytes |
| `full_name` | string | Sí | No vacío, ≤255 chars |

**Response 201:**

```json
{
  "id": "fad7cbd3-0970-46fa-aaba-b9474a65a10c",
  "email": "juan@example.com",
  "full_name": "Juan Pérez",
  "alias": "juan",
  "status": "ACTIVE"
}
```

**Errores:**

| HTTP | Code | Cuándo |
|---|---|---|
| 400 | `INVALID_REQUEST` | JSON malformado o campos desconocidos |
| 400 | `EMAIL_REQUIRED` | Email vacío |
| 400 | `EMAIL_INVALID` | Formato inválido |
| 400 | `EMAIL_TOO_LONG` | >255 chars |
| 400 | `FULL_NAME_REQUIRED` | Nombre vacío |
| 400 | `PASSWORD_TOO_SHORT` | <8 bytes |
| 400 | `PASSWORD_TOO_LONG` | >72 bytes |
| 409 | `EMAIL_EXISTS` | Email ya registrado |
| 409 | `ALIAS_EXISTS` | Colisión de alias (raro, reintentar) |
| 500 | `INTERNAL_ERROR` | Error de TB o DB |

**Notas:**
- El `alias` se genera automáticamente del email (`juan@example.com` → `juan`).
- Si `juan` está tomado, se usa `juan2`, `juan3`, etc.
- El usuario arranca con `status=ACTIVE` si TB responde; si no, queda `PENDING` y el reconciliador lo recupera.

---

### 3.2 `POST /api/auth/login`

Autentica un usuario y devuelve un JWT.

**Auth:** No requerida.

**Headers:** `Content-Type: application/json`.

**Request body:**

```json
{
  "email": "juan@example.com",
  "password": "TestPassword123!"
}
```

**Response 200:**

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": "fad7cbd3-0970-46fa-aaba-b9474a65a10c",
    "email": "juan@example.com",
    "full_name": "Juan Pérez",
    "alias": "juan",
    "status": "ACTIVE"
  }
}
```

**Payload del JWT (decodificado):**

```json
{
  "sub": "fad7cbd3-0970-46fa-aaba-b9474a65a10c",
  "tb_account_id": "c442931df9fcfd81bc6f7e3e3b2ecbb1",
  "iss": "banking-system",
  "iat": 1789513239,
  "exp": 1789599639
}
```

**El JWT contiene:**

- `sub`: UUID del usuario (identidad de aplicación).
- `tb_account_id`: hex del uint128 (identidad financiera).
- `iss`: emisor (`banking-system`).
- `iat` / `exp`: timestamps de emisión y expiración.

**Nota para el frontend:** decodificar el JWT al recibirlo (sin verificar firma, solo leer el payload) permite tener ambos IDs disponibles sin fetches adicionales. El `tb_account_id` es necesario para calcular la dirección de las transacciones en `/api/transactions/history`.

**Ejemplo:**

```js
function decodeJWT(token) {
  const payload = token.split('.')[1];
  return JSON.parse(atob(payload));
}
// → { sub: "...", tb_account_id: "...", exp: ..., iat: ..., iss: "..." }
```

**Errores:**

| HTTP | Code | Cuándo |
|---|---|---|
| 400 | `INVALID_REQUEST` | JSON malformado |
| 400 | `EMAIL_REQUIRED` | Email vacío |
| 400 | `PASSWORD_REQUIRED` | Password vacía |
| 401 | `INVALID_CREDENTIALS` | Email inexistente **o** password incorrecta |
| 401 | `ACCOUNT_PENDING` | Password OK pero status PENDING |

**Notas:**
- **Anti-enumeración:** los errores de email inexistente y password incorrecta son idénticos.
- El token expira en 24h (`JWT_EXPIRY_HOURS=24`).

---

### 3.3 `POST /api/auth/logout`

Cierra la sesión del usuario.

**Auth:** No requerida.

**Request body:** vacío.

**Response 204:** sin body.

**Notas:**
- **Stateless:** el backend no mantiene blacklist de tokens.
- **Responsabilidad del cliente:** borrar el token de `localStorage`.
- **Idempotente:** llamarlo con o sin token, siempre devuelve 204.
- **Importante:** el token sigue siendo válido hasta expirar. Si el cliente no lo borra, sigue funcionando.

---

### 3.4 `GET /api/auth/me`

Devuelve los datos del usuario autenticado.

**Auth:** JWT requerido.

**Headers:** `Authorization: Bearer <jwt>`.

**Response 200:**

```json
{
  "id": "fad7cbd3-0970-46fa-aaba-b9474a65a10c",
  "email": "juan@example.com",
  "full_name": "Juan Pérez",
  "alias": "juan",
  "status": "ACTIVE"
}
```

**Errores:**

| HTTP | Code | Cuándo |
|---|---|---|
| 401 | `TOKEN_REQUIRED` | Sin header Authorization |
| 401 | `TOKEN_INVALID` | JWT malformado o firma inválida |
| 401 | `TOKEN_EXPIRED` | JWT expirado |
| 404 | `USER_NOT_FOUND` | Usuario eliminado |

---

## 4. Cuentas

### 4.1 `GET /api/account`

Devuelve la información completa de la cuenta del usuario.

**Auth:** JWT requerido.

**Response 200:**

```json
{
  "tb_account_id": "c442931df9fcfd81bc6f7e3e3b2ecbb1",
  "balance_cents": 106500,
  "ledger": 1,
  "code": 100
}
```

| Campo | Tipo | Descripción |
|---|---|---|
| `tb_account_id` | string | ID de la cuenta en TigerBeetle (hex 32) |
| `balance_cents` | int64 | Saldo en centavos |
| `ledger` | uint32 | Ledger contable (siempre 1 = USD) |
| `code` | uint16 | Tipo de cuenta (100 = savings) |

**Nota:** este endpoint NO incluye `full_name`, `email`, ni `alias`. Esos datos vienen de `GET /api/auth/me`.

**Para construir el dashboard, el frontend tiene dos opciones:**

**Opción A — Dos fetches:**

```js
const [me, account] = await Promise.all([
  api.get('/api/auth/me'),
  api.get('/api/account'),
]);
```

**Opción B — Un fetch + JWT decode:**

```js
const me = await api.get('/api/auth/me');
const claims = decodeJWT(localStorage.getItem('token'));
// claims.tb_account_id ya está disponible
const balance = await api.get('/api/account/balance');
```

**Recomendación:** guardar `user` (de `/api/auth/me`) y `tb_account_id` (del JWT decode) en el `AuthContext`. El saldo se consulta por separado cuando se necesite.

**Errores:**

| HTTP | Code | Cuándo |
|---|---|---|
| 401 | `TOKEN_REQUIRED` / `TOKEN_INVALID` / `TOKEN_EXPIRED` | Problema con JWT |
| 403 | `USER_NOT_ACTIVE` | Usuario PENDING |
| 404 | `ACCOUNT_NOT_FOUND` | Cuenta TB no existe (raro) |

---

### 4.2 `GET /api/account/balance`

Devuelve solo el saldo en centavos.

**Auth:** JWT requerido.

**Response 200:**

```json
{
  "balance_cents": 106500
}
```

**Notas:**
- Más liviano que `/api/account` cuando solo se necesita el saldo.
- El saldo se calcula como `credits_posted - debits_posted` en TigerBeetle.

---

## 5. Transacciones

### 5.1 `POST /api/transactions/deposit`

Deposita dinero en la cuenta del usuario (desde el banco).

**Auth:** JWT requerido.

**Headers:**
- `Content-Type: application/json`
- `Idempotency-Key: <uuid>` (opcional, recomendado)

**Request body:**

```json
{
  "amount_cents": 10000
}
```

| Campo | Tipo | Requerido | Validación |
|---|---|---|---|
| `amount_cents` | int64 | Sí | > 0 |

**Response 201:**

```json
{
  "id": "3ec93dd7-527c-4b84-bc69-ec0bf1b9c387",
  "debit_account_id": "01000000000000000000000000000000",
  "credit_account_id": "c442931df9fcfd81bc6f7e3e3b2ecbb1",
  "amount_cents": 10000,
  "code": 1,
  "created_at": "2026-09-15T01:46:55.830277Z"
}
```

| Campo | Descripción |
|---|---|
| `id` | UUID de la transacción en `transactions_log` |
| `debit_account_id` | Cuenta origen (banco, ID=1) |
| `credit_account_id` | Cuenta destino (usuario) |
| `amount_cents` | Monto en centavos |
| `code` | 1 = depósito |
| `created_at` | Timestamp UTC |

**Errores:**

| HTTP | Code | Cuándo |
|---|---|---|
| 400 | `INVALID_AMOUNT` | Monto ≤ 0 |
| 401 | `TOKEN_*` | Problema con JWT |
| 403 | `USER_NOT_ACTIVE` | Usuario PENDING |

**Idempotencia:**
- Si se envía `Idempotency-Key`, la operación es idempotente.
- Mismo key → mismo `TBTransferID` → misma transacción → mismo `id`.
- Sin key: se genera un ID único con timestamp + random.

---

### 5.2 `POST /api/transactions/withdraw`

Retira dinero de la cuenta del usuario (hacia el banco).

**Auth:** JWT requerido.

**Headers:** igual que deposit.

**Request body:**

```json
{
  "amount_cents": 3000
}
```

**Response 201:**

```json
{
  "id": "7e66811d-6e03-429d-b61a-2e162202a060",
  "debit_account_id": "c442931df9fcfd81bc6f7e3e3b2ecbb1",
  "credit_account_id": "01000000000000000000000000000000",
  "amount_cents": 3000,
  "code": 2,
  "created_at": "2026-09-15T01:42:46.784506Z"
}
```

**Diferencias con deposit:**
- `debit_account_id` = usuario (sale del usuario)
- `credit_account_id` = banco (entra al banco)
- `code` = 2 (withdrawal)

**Errores:**

| HTTP | Code | Cuándo |
|---|---|---|
| 400 | `INVALID_AMOUNT` | Monto ≤ 0 |
| 400 | `INSUFFICIENT_FUNDS` | Saldo insuficiente |
| 403 | `USER_NOT_ACTIVE` | Usuario PENDING |

**Nota sobre `INSUFFICIENT_FUNDS`:** doble validación.
1. El backend consulta el saldo antes.
2. TigerBeetle rechaza con `TransferExceedsCredits` si el saldo no alcanza.
3. El segundo es la garantía real (concurrencia).

---

### 5.3 `POST /api/transactions/transfer`

Transfiere dinero a otra cuenta.

**Auth:** JWT requerido.

**Headers:** igual que deposit.

**Request body:**

```json
{
  "to_account_id": "4c5d789e69ae0ad5f3d630bc5931623e",
  "amount_cents": 5000
}
```

| Campo | Tipo | Requerido | Validación |
|---|---|---|---|
| `to_account_id` | string | Sí | Hex de 32 chars (uint128) |
| `amount_cents` | int64 | Sí | > 0 |

**Response 201:**

```json
{
  "id": "334b5b62-7456-4145-9576-c60bacdbcebb",
  "debit_account_id": "c442931df9fcfd81bc6f7e3e3b2ecbb1",
  "credit_account_id": "4c5d789e69ae0ad5f3d630bc5931623e",
  "amount_cents": 5000,
  "code": 3,
  "created_at": "2026-09-15T16:35:00Z"
}
```

**Errores:**

| HTTP | Code | Cuándo |
|---|---|---|
| 400 | `INVALID_AMOUNT` | Monto ≤ 0 |
| 400 | `INVALID_DEST_ACCOUNT` | Hex malformado |
| 400 | `DEST_NOT_FOUND` | Cuenta destino no existe en PG |
| 400 | `DEST_NOT_ACTIVE` | Cuenta destino en PENDING |
| 400 | `SAME_ACCOUNT` | Origen = destino |
| 400 | `INSUFFICIENT_FUNDS` | Saldo insuficiente |
| 403 | `USER_NOT_ACTIVE` | Usuario origen PENDING |

**Validaciones en orden:**
1. Monto > 0.
2. Hex del destino válido.
3. Usuario origen existe y está ACTIVE.
4. Usuario destino existe en PostgreSQL.
5. Usuario destino está ACTIVE.
6. Origen ≠ destino.
7. TigerBeetle verifica saldo.

**Cómo obtener el `to_account_id`:**
- `GET /api/users/lookup?email=...` → devuelve `tb_account_id`.
- `GET /api/users/lookup?alias=...` → devuelve `tb_account_id`.

---

### 5.4 `GET /api/transactions/history`

Devuelve el historial paginado del usuario.

**Auth:** JWT requerido.

**Query params:**

| Param | Tipo | Default | Validación |
|---|---|---|---|
| `page` | int | 1 | ≥ 1 |
| `limit` | int | 20 | 1-100 |

**Ejemplo:**

```
GET /api/transactions/history?page=1&limit=10
```

**Response 200:**

```json
{
  "transactions": [
    {
      "id": "7e66811d-6e03-429d-b61a-2e162202a060",
      "debit_account_id": "c442931df9fcfd81bc6f7e3e3b2ecbb1",
      "credit_account_id": "01000000000000000000000000000000",
      "amount_cents": 3000,
      "code": 2,
      "created_at": "2026-09-15T01:42:46.784506Z"
    },
    {
      "id": "62cad0c5-d53c-433f-b508-8d1660b25a09",
      "debit_account_id": "01000000000000000000000000000000",
      "credit_account_id": "c442931df9fcfd81bc6f7e3e3b2ecbb1",
      "amount_cents": 10000,
      "code": 1,
      "created_at": "2026-09-15T01:42:37.492742Z"
    }
  ],
  "total": 2,
  "page": 1,
  "limit": 10,
  "total_pages": 1
}
```

**Códigos de transacción:**

| Code | Tipo |
|---|---|
| 1 | Depósito |
| 2 | Retiro |
| 3 | Transferencia |

**Ordenamiento:** `created_at DESC` (más recientes primero).

**Dirección de la transacción:**

Cada transacción tiene `debit_account_id` y `credit_account_id` crudos. El frontend debe calcular la dirección comparando contra el `tb_account_id` del usuario autenticado.

**Regla:**
- Si `credit_account_id === userTBAccountID` → **`in`** (recibió dinero).
- Si `debit_account_id === userTBAccountID` → **`out`** (envió dinero).

**Ejemplo:**

```js
function computeDirection(tx, userTBAccountID) {
  if (tx.credit_account_id === userTBAccountID) return 'in';
  if (tx.debit_account_id === userTBAccountID) return 'out';
  return 'unknown';
}

// En el componente:
const userTBAccountID = useAuth().tbAccountID; // del JWT decode

txs.map(tx => ({
  ...tx,
  direction: computeDirection(tx, userTBAccountID),
}));
```

**Cómo obtener `userTBAccountID`:**

- **Opción A:** decodificar el JWT al hacer login y guardarlo en `AuthContext`.
- **Opción B:** llamar a `GET /api/auth/me` y usar el `tb_account_id`... **pero `/api/auth/me` no lo devuelve actualmente** — solo devuelve `id`, `email`, `full_name`, `alias`, `status`.

**La opción A es la única viable.** El `tb_account_id` solo está en el JWT.

**Nota:** el backend NO devuelve `direction` calculado porque la misma transacción es `out` para el remitente e `in` para el destinatario. Calcularlo en el backend requeriría saber "desde el punto de vista de quién", que solo el frontend puede determinar.

---

### 5.5 `POST /api/transactions/demo-topup`

Carga $1000 de demo a la cuenta del usuario.

**Auth:** JWT requerido.

**Solo disponible en `APP_ENV=development`.**

**Request body:** vacío.

**Response 201:**

```json
{
  "amount_cents": 100000,
  "message": "Crédito de demo cargado",
  "transaction": {
    "id": "4fb3a6f1-d6f4-4823-85fe-fa58ccce579a",
    "debit_account_id": "01000000000000000000000000000000",
    "credit_account_id": "1f879f4d2089a5fd56b8233048af9dbe",
    "amount_cents": 100000,
    "code": 1,
    "created_at": "2026-09-15T21:41:38.694109Z"
  }
}
```

**Errores:**

| HTTP | Code | Cuándo |
|---|---|---|
| 403 | `DEMO_DISABLED` | `APP_ENV != development` |

**Idempotencia:** usa una key fija (`"demo-topup"`) por usuario. Dos llamadas seguidas solo cargan $1000 una vez.

**Uso:** permite al evaluador probar el sistema sin depositar primero.

---

## 6. Chat IA

### 6.1 `POST /api/chat`

Envía un mensaje al asistente de IA.

**Auth:** JWT requerido.

**Headers:** `Content-Type: application/json`.

**Request body:**

```json
{
  "message": "¿Cuánto tengo?",
  "history": [
    {"role": "user", "content": "Hola"},
    {"role": "assistant", "content": "Hola, ¿en qué puedo ayudarte?"}
  ]
}
```

| Campo | Tipo | Requerido | Validación |
|---|---|---|---|
| `message` | string | Sí | No vacío |
| `history` | array<Message> | No | Últimos 20 mensajes |

**Message:**

```json
{
  "role": "user" | "assistant",
  "content": "texto"
}
```

**Response 200 — consulta normal:**

```json
{
  "reply": "Tienes un saldo actual de $65.00 (6,500 centavos).",
  "tool_calls": [
    {
      "id": "call_75fff60dc9124ead8abeb00c",
      "type": "function",
      "function": {
        "name": "get_balance",
        "arguments": "{}"
      }
    }
  ],
  "usage": {
    "prompt_tokens": 2003,
    "completion_tokens": 56,
    "total_tokens": 2059
  }
}
```

**Response 200 — con confirmación pendiente:**

```json
{
  "reply": "Voy a depositar $50.00. ¿Confirmas?",
  "tool_calls": [
    {
      "id": "call_44e46764d73d4001b25962be",
      "type": "function",
      "function": {
        "name": "deposit",
        "arguments": "{\"amount_cents\": 5000}"
      }
    }
  ],
  "pending_confirmation": {
    "confirmation_token": "4804d83e-a526-4c8c-b53e-0faa1a12252e",
    "type": "deposit",
    "amount_cents": 5000,
    "to_account_id": "",
    "expires_at": "2026-09-15T18:10:40Z"
  },
  "usage": {
    "prompt_tokens": 2121,
    "completion_tokens": 286,
    "total_tokens": 2407
  }
}
```

**Campos de la respuesta:**

| Campo | Descripción |
|---|---|
| `reply` | Texto del asistente para mostrar al usuario |
| `tool_calls` | Tools que el modelo ejecutó (puede ser `null`) |
| `pending_confirmation` | **Solo si hay una operación pendiente** (puede ser `null`) |
| `usage` | Conteo de tokens del LLM |

**`pending_confirmation`:**

| Campo | Tipo | Descripción |
|---|---|---|
| `confirmation_token` | string | UUID que se envía a `/api/chat/confirm` |
| `type` | string | `deposit`, `withdraw`, o `transfer` |
| `amount_cents` | int64 | Monto de la operación |
| `to_account_id` | string | **Solo para `type=transfer`**. Hex 32 chars. Vacío (`""`) para `deposit` y `withdraw`. |
| `expires_at` | string | ISO 8601 UTC |

**⚠️ Importante para el frontend:** cuando `type` es `deposit` o `withdraw`, `to_account_id` viene como string vacío (`""`). **No renderizarlo como "cuenta destino".** Solo mostrarlo si `type === "transfer"`.

**Ejemplo de UI:**

```jsx
function ConfirmationCard({ pending }) {
  return (
    <div>
      <p>Operación: {pending.type}</p>
      <p>Monto: ${(pending.amount_cents / 100).toFixed(2)}</p>
      {pending.type === 'transfer' && pending.to_account_id && (
        <p>Destino: {pending.to_account_id}</p>
      )}
      <button onClick={handleConfirm}>Confirmar</button>
      <button onClick={handleCancel}>Cancelar</button>
    </div>
  );
}
```

**Tools expuestas al modelo:**

| Tool | Args | Descripción |
|---|---|---|
| `get_balance` | — | Saldo actual |
| `get_history` | `limit` (1-50) | Últimas N transacciones |
| `deposit` | `amount_cents` | Depositar (requiere confirmación) |
| `withdraw` | `amount_cents` | Retirar (requiere confirmación) |
| `transfer` | `to_account_id`, `amount_cents` | Transferir (requiere confirmación) |

**Errores:**

| HTTP | Code | Cuándo |
|---|---|---|
| 400 | `EMPTY_MESSAGE` | Mensaje vacío |
| 400 | `INVALID_REQUEST` | JSON malformado |
| 401 | `TOKEN_*` | Problema con JWT |
| 500 | `INTERNAL_ERROR` | Error de OpenRouter o LLM |

**Flujo esperado del frontend:**

1. Usuario escribe "Deposita $50".
2. Frontend → `POST /api/chat`.
3. Backend responde con `pending_confirmation`.
4. Frontend muestra los botones "Confirmar" / "Cancelar".
5. **Si confirma:** `POST /api/chat/confirm` con el token.
6. **Si cancela:** limpia el estado local. **No** llama al backend.

---

### 6.2 `POST /api/chat/confirm`

Ejecuta una operación pendiente de confirmación.

**Auth:** JWT requerido.

**Headers:** `Content-Type: application/json`.

**Request body:**

```json
{
  "confirmation_token": "4804d83e-a526-4c8c-b53e-0faa1a12252e"
}
```

| Campo | Tipo | Requerido |
|---|---|---|
| `confirmation_token` | string | Sí |

**Response 200:**

```json
{
  "success": true,
  "message": "Operación 'deposit' ejecutada correctamente",
  "operation": "deposit"
}
```

**Errores:**

| HTTP | Code | Cuándo |
|---|---|---|
| 400 | `MISSING_TOKEN` | Body sin `confirmation_token` |
| 400 | `INVALID_REQUEST` | JSON malformado |
| 404 | `CONFIRMATION_NOT_FOUND` | Token inválido, expirado, usado, o de otro usuario |
| 401 | `TOKEN_*` | Problema con JWT |

**Garantías del token:**

- **Un solo uso:** una vez consumido, `used_at` se marca y no se puede reusar.
- **Expiración:** 5 minutos desde su creación.
- **Dueño:** solo el usuario que lo creó puede usarlo.
- **Atómico:** la validación + consumo es una sola query (`UPDATE ... WHERE`).

**Nota:** `CONFIRMATION_NOT_FOUND` es **deliberadamente genérico**. No se distingue entre "token inválido", "expirado", "ya usado" o "de otro usuario" para no filtrar información.

---

## 7. Utilidades

### 7.1 `GET /api/users/lookup`

Busca un usuario por email o alias. Se usa para transferencias.

**Auth:** JWT requerido.

**Query params (exactamente uno):**

| Param | Tipo | Descripción |
|---|---|---|
| `email` | string | Email del usuario |
| `alias` | string | Alias del usuario |

**Ejemplos:**

```
GET /api/users/lookup?email=demo2@banco.com
GET /api/users/lookup?alias=demo2
```

**Response 200:**

```json
{
  "user_id": "66920974-eff9-4694-9ef9-504bae22559e",
  "email": "demo2@banco.com",
  "full_name": "Usuario Demo 2",
  "alias": "demo2",
  "tb_account_id": "0d041711f704fad272c80ada36fcbf2b"
}
```

| Campo | Uso |
|---|---|
| `user_id` | UUID interno (poco útil en frontend) |
| `email` | Mostrar en confirmación |
| `full_name` | Mostrar en confirmación |
| `alias` | Mostrar en confirmación |
| `tb_account_id` | **El que se envía a `POST /api/transactions/transfer`** |

**Errores:**

| HTTP | Code | Cuándo |
|---|---|---|
| 400 | `MISSING_QUERY` | Ni email ni alias |
| 400 | `AMBIGUOUS_QUERY` | Ambos email y alias |
| 404 | `USER_NOT_FOUND` | No existe o no está ACTIVE |
| 401 | `TOKEN_*` | Problema con JWT |

**Notas:**
- Solo devuelve usuarios **ACTIVE**.
- No expone `password_hash`.

---

### 7.2 `GET /health`

Healthcheck del sistema.

**Auth:** No requerida.

**Response 200 (todo OK):**

```json
{
  "status": "healthy",
  "postgres": "ok",
  "tigerbeetle": "ok"
}
```

**Response 503 (algo caído):**

```json
{
  "status": "degraded",
  "postgres": "ok",
  "tigerbeetle": "error"
}
```

**Semántica:**

| Campo | Valores | Qué verifica |
|---|---|---|
| `postgres` | `ok` / `error` | `Ping()` a PostgreSQL |
| `tigerbeetle` | `ok` / `error` | `AccountExists(BankAccountID)` en TB |
| `status` | `healthy` / `degraded` | Ambos anteriores |

**Notas:**
- El healthcheck de TB usa `LookupAccounts`, no `Nop()`. Tarda 2s max si TB está caído.
- Se puede usar con orquestadores (Kubernetes, Docker Swarm).

---

## 8. Tabla resumen

| # | Método | Endpoint | Auth | Body | Response exitosa |
|---|---|---|---|---|---|
| 1 | POST | `/api/auth/register` | No | `{email, password, full_name}` | 201 `{id, email, full_name, alias, status}` |
| 2 | POST | `/api/auth/login` | No | `{email, password}` | 200 `{token, user}` |
| 3 | POST | `/api/auth/logout` | No | — | 204 |
| 4 | GET | `/api/auth/me` | JWT | — | 200 `{id, email, full_name, alias, status}` |
| 5 | GET | `/api/account` | JWT | — | 200 `{tb_account_id, balance_cents, ledger, code}` |
| 6 | GET | `/api/account/balance` | JWT | — | 200 `{balance_cents}` |
| 7 | POST | `/api/transactions/deposit` | JWT | `{amount_cents}` | 201 `Transaction` |
| 8 | POST | `/api/transactions/withdraw` | JWT | `{amount_cents}` | 201 `Transaction` |
| 9 | POST | `/api/transactions/transfer` | JWT | `{to_account_id, amount_cents}` | 201 `Transaction` |
| 10 | GET | `/api/transactions/history` | JWT | Query: `?page=1&limit=20` | 200 `HistoryResult` |
| 11 | POST | `/api/transactions/demo-topup` | JWT | — | 201 `DemoTopupResult` |
| 12 | POST | `/api/chat` | JWT | `{message, history?}` | 200 `ChatResult` |
| 13 | POST | `/api/chat/confirm` | JWT | `{confirmation_token}` | 200 `ConfirmResult` |
| 14 | GET | `/api/users/lookup` | JWT | Query: `?email=X` o `?alias=Y` | 200 `UserLookup` |
| 15 | GET | `/health` | No | — | 200 / 503 |

---

## 9. Códigos de error

**Lista completa, agrupada por dominio:**

### Autenticación

| Code | HTTP | Descripción |
|---|---|---|
| `INVALID_REQUEST` | 400 | JSON malformado |
| `EMAIL_REQUIRED` | 400 | Email obligatorio |
| `EMAIL_INVALID` | 400 | Formato inválido |
| `EMAIL_TOO_LONG` | 400 | >255 chars |
| `FULL_NAME_REQUIRED` | 400 | Nombre obligatorio |
| `PASSWORD_REQUIRED` | 400 | Password obligatoria |
| `PASSWORD_TOO_SHORT` | 400 | <8 bytes |
| `PASSWORD_TOO_LONG` | 400 | >72 bytes |
| `EMAIL_EXISTS` | 409 | Email duplicado |
| `ALIAS_EXISTS` | 409 | Alias duplicado (reintentar) |
| `INVALID_CREDENTIALS` | 401 | Login fallido |
| `ACCOUNT_PENDING` | 401 | Usuario PENDING |

### JWT / Sesión

| Code | HTTP | Descripción |
|---|---|---|
| `TOKEN_REQUIRED` | 401 | Sin Authorization |
| `TOKEN_INVALID` | 401 | JWT malformado o firma inválida |
| `TOKEN_EXPIRED` | 401 | JWT expirado |
| `TOKEN_INVALID_CLAIMS` | 401 | Claims malformados |

### Usuarios / Cuentas

| Code | HTTP | Descripción |
|---|---|---|
| `USER_NOT_FOUND` | 404 | Usuario no existe |
| `USER_NOT_ACTIVE` | 403 | Usuario PENDING |
| `ACCOUNT_NOT_FOUND` | 404 | Cuenta TB no existe |

### Transacciones

| Code | HTTP | Descripción |
|---|---|---|
| `INVALID_AMOUNT` | 400 | Monto ≤ 0 |
| `INVALID_DEST_ACCOUNT` | 400 | Hex malformado |
| `DEST_NOT_FOUND` | 400 | Destino no existe |
| `DEST_NOT_ACTIVE` | 400 | Destino PENDING |
| `SAME_ACCOUNT` | 400 | Origen = destino |
| `INSUFFICIENT_FUNDS` | 400 | Saldo insuficiente |

### Lookup

| Code | HTTP | Descripción |
|---|---|---|
| `MISSING_QUERY` | 400 | Sin email ni alias |
| `AMBIGUOUS_QUERY` | 400 | Ambos email y alias |

### Chat / Confirmación

| Code | HTTP | Descripción |
|---|---|---|
| `EMPTY_MESSAGE` | 400 | Mensaje vacío |
| `MISSING_TOKEN` | 400 | Sin confirmation_token |
| `CONFIRMATION_NOT_FOUND` | 404 | Token inválido/expirado/usado |
| `UNKNOWN_TOOL` | 400 | Tool desconocida |
| `MISSING_AMOUNT` | 400 | Falta amount_cents |
| `MISSING_TO_ACCOUNT_ID` | 400 | Falta to_account_id |

### Demo

| Code | HTTP | Descripción |
|---|---|---|
| `DEMO_DISABLED` | 403 | APP_ENV != development |

### Sistema

| Code | HTTP | Descripción |
|---|---|---|
| `INTERNAL_ERROR` | 500 | Error interno (loggeado) |
| `UNIQUE_VIOLATION` | 409 | Constraint de DB |

---

## 10. Ejemplos end-to-end

### 10.1 Flujo completo: registro → login → depósito → transferencia

**1. Registro:**

```bash
curl -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "juan@example.com",
    "password": "TestPassword123!",
    "full_name": "Juan Pérez"
  }'
```

**Response 201:**

```json
{
  "id": "fad7cbd3-...",
  "email": "juan@example.com",
  "full_name": "Juan Pérez",
  "alias": "juan",
  "status": "ACTIVE"
}
```

**2. Login:**

```bash
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"juan@example.com","password":"TestPassword123!"}'
```

**Response 200:**

```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "user": {...}
}
```

**3. Guardar el token:**

```bash
TOKEN="eyJhbGciOiJIUzI1NiIs..."
```

**4. Depositar $100:**

```bash
curl -X POST http://localhost:8080/api/transactions/deposit \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{"amount_cents": 10000}'
```

**5. Consultar saldo:**

```bash
curl -X GET http://localhost:8080/api/account/balance \
  -H "Authorization: Bearer $TOKEN"
# → {"balance_cents": 10000}
```

**6. Buscar destinatario:**

```bash
curl -X GET "http://localhost:8080/api/users/lookup?email=demo2@banco.com" \
  -H "Authorization: Bearer $TOKEN"
# → {"user_id": "...", "tb_account_id": "0d041711f704fad272c80ada36fcbf2b", ...}
```

**7. Transferir $30:**

```bash
curl -X POST http://localhost:8080/api/transactions/transfer \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "to_account_id": "0d041711f704fad272c80ada36fcbf2b",
    "amount_cents": 3000
  }'
```

**Response 201:**

```json
{
  "id": "334b5b62-...",
  "debit_account_id": "c442931d...",
  "credit_account_id": "0d041711...",
  "amount_cents": 3000,
  "code": 3,
  "created_at": "2026-09-15T16:35:00Z"
}
```

**8. Ver historial:**

```bash
curl -X GET "http://localhost:8080/api/transactions/history?page=1&limit=10" \
  -H "Authorization: Bearer $TOKEN"
```

---

### 10.2 Flujo completo: chat → confirmación → ejecución

**1. Usuario pregunta:**

```bash
curl -X POST http://localhost:8080/api/chat \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"message":"Deposita $50 a mi cuenta"}'
```

**Response 200:**

```json
{
  "reply": "Voy a depositar $50.00 (5000 centavos) a tu cuenta. ¿Confirmas?",
  "tool_calls": [{"id": "call_abc", "type": "function",
                  "function": {"name": "deposit", "arguments": "{\"amount_cents\": 5000}"}}],
  "pending_confirmation": {
    "confirmation_token": "4804d83e-a526-4c8c-b53e-0faa1a12252e",
    "type": "deposit",
    "amount_cents": 5000,
    "to_account_id": "",
    "expires_at": "2026-09-15T18:10:40Z"
  },
  "usage": {...}
}
```

**2. Frontend muestra los botones de confirmación.**

**3. Usuario confirma:**

```bash
curl -X POST http://localhost:8080/api/chat/confirm \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"confirmation_token": "4804d83e-a526-4c8c-b53e-0faa1a12252e"}'
```

**Response 200:**

```json
{
  "success": true,
  "message": "Operación 'deposit' ejecutada correctamente",
  "operation": "deposit"
}
```

**4. El saldo se actualiza automáticamente** (el frontend debe consultarlo o mostrarlo ya actualizado).

---

## 11. Notas para el frontend

### 11.1 AuthContext — estructura recomendada

```jsx
const AuthContext = createContext();

function AuthProvider({ children }) {
  const [user, setUser] = useState(null);         // de /api/auth/me
  const [tbAccountID, setTbAccountID] = useState(null); // del JWT decode
  const [token, setToken] = useState(null);

  async function login(email, password) {
    const { data } = await api.post('/api/auth/login', { email, password });
    const claims = decodeJWT(data.token);

    localStorage.setItem('token', data.token);
    setToken(data.token);
    setUser(data.user);
    setTbAccountID(claims.tb_account_id);
  }

  function logout() {
    localStorage.removeItem('token');
    setToken(null);
    setUser(null);
    setTbAccountID(null);
  }

  // Al montar, restaurar sesión desde localStorage
  useEffect(() => {
    const t = localStorage.getItem('token');
    if (t) {
      const claims = decodeJWT(t);
      setToken(t);
      setTbAccountID(claims.tb_account_id);
      api.get('/api/auth/me').then(({ data }) => setUser(data));
    }
  }, []);

  return (
    <AuthContext.Provider value={{ user, tbAccountID, token, login, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

function decodeJWT(token) {
  try {
    return JSON.parse(atob(token.split('.')[1]));
  } catch {
    return {};
  }
}
```

### 11.2 Componentes que necesitan `tbAccountID`

| Componente | Para qué |
|---|---|
| `History.jsx` | Calcular dirección (`in`/`out`) de cada transacción |
| `Dashboard.jsx` | Mostrar el ID de la cuenta del usuario |
| `Transactions.jsx` | Detectar si está transfiriendo a su propia cuenta (validación local opcional) |

**No necesitan `tbAccountID`:**

- `Login.jsx`, `Register.jsx`: solo usan email/password.
- `Chat.jsx`: el backend maneja el `tb_account_id` del usuario internamente.
- `BalanceCard.jsx`: usa `/api/account/balance` que ya devuelve solo el saldo.

### 11.3 Consideración sobre `/api/account`

**`/api/account` no devuelve `email`, `full_name`, ni `alias`.** Los datos del usuario vienen de `/api/auth/me`.

**Recomendación:** en el Dashboard, hacer un solo fetch de `/api/account/balance` (más liviano) y usar `user` y `tbAccountID` del `AuthContext` para el resto.

```jsx
function Dashboard() {
  const { user, tbAccountID } = useAuth();
  const [balance, setBalance] = useState(null);

  useEffect(() => {
    api.get('/api/account/balance').then(({ data }) => setBalance(data.balance_cents));
  }, []);

  return (
    <>
      <h1>Hola, {user.full_name}</h1>
      <p>Alias: @{user.alias}</p>
      <p>Saldo: ${(balance / 100).toFixed(2)}</p>
      <p>Tu cuenta: {tbAccountID}</p>
    </>
  );
}
```

### 11.4 Manejo de errores

```js
try {
  const { data } = await api.post('/api/transactions/transfer', {...});
  // Éxito
} catch (err) {
  const { code, message } = err.response?.data || {};
  // Mostrar `message` al usuario
  // Usar `code` para lógica específica si es necesario
}
```

### 11.5 Idempotencia en escrituras

**Siempre enviar `Idempotency-Key`** en deposit, withdraw y transfer:

```js
const idempotencyKey = crypto.randomUUID();
await api.post('/api/transactions/transfer', {...}, {
  headers: { 'Idempotency-Key': idempotencyKey }
});
```

Si la request falla, **reintentar con la misma key** para no duplicar.

### 11.6 Confirmación en chat

**El "Sí" del usuario NO se envía a `/api/chat`.** Se envía a `/api/chat/confirm`.

```js
// Al recibir respuesta de /api/chat
if (data.pending_confirmation) {
  // Guardar el token en estado
  setPending(data.pending_confirmation);
}

// Al hacer clic en "Confirmar"
async function handleConfirm() {
  await api.post('/api/chat/confirm', {
    confirmation_token: pending.confirmation_token
  });
  setPending(null);
}

// Al hacer clic en "Cancelar"
function handleCancel() {
  setPending(null); // solo limpia el estado local
}
```

### 11.7 Formato de fechas

Todos los timestamps vienen en ISO 8601 UTC. El frontend debe formatearlos localmente.

```js
function formatDate(iso) {
  return new Date(iso).toLocaleString('es-ES', {
    day: '2-digit',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}
// → "15 sep 2026, 18:35"
```

### 11.8 Montos

**Todos los montos vienen en centavos como `int64`.** El frontend convierte:

```js
const display = `$${(cents / 100).toFixed(2)}`;
const inputToCents = (userInput) => Math.round(parseFloat(userInput) * 100);
```

**Nunca usar `float64` para manipular dinero.** Solo en el input del usuario y en el display final.

---

**Fin del documento.**

Este documento es la **fuente de verdad** para la API. Cualquier discrepancia con el código debe resolverse actualizando este documento o el código, no ambos por separado.

**Última actualización:** 15 de septiembre de 2026
**Versión:** 1.0
**Mantenedor:** Gean Munoz