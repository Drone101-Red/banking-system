# 🎨 Frontend — Documentación Técnica Completa

**Proyecto:** Banking System — Frontend
**Stack:** React 19 + Vite 8 + Tailwind 3.4
**Fecha:** 15 de septiembre de 2026
**Estado:** Completo y funcional

---

## 1. Resumen

El frontend es una SPA (Single Page Application) en **React 19** con **Vite 8** y **Tailwind CSS 3.4**. Consume los 15 endpoints del backend mediante **axios** con interceptores JWT. Se sirve desde **nginx** en Docker y se comunica con el backend usando el hostname interno de Docker.

**Se construyó en 6 batches incrementales, cada uno compilando antes del siguiente.**

---

## 2. Stack tecnológico

| Capa | Tecnología | Versión | Rol |
|---|---|---|---|
| Framework | React | 19.2.8 | UI |
| Build tool | Vite | 8.3.0 | Dev server + build |
| Bundler | Rolldown | (incluido en Vite 8) | Build rápido |
| Routing | react-router-dom | 7.18.4 | Navegación SPA |
| Estilos | Tailwind CSS | 3.4.19 | Utility-first CSS |
| HTTP | axios | 1.20.0 | Cliente HTTP |
| Iconos | lucide-react | 1.46.0 | Iconografía |
| Linter | Oxlint | 1.81.0 | Linter (reemplazo moderno de ESLint) |
| Servidor | nginx | alpine | Servir el build en producción |

---

## 3. Estructura de carpetas

```
frontend/
├── Dockerfile                  # Multi-stage: node:20 + nginx:alpine
├── nginx.conf                  # SPA fallback + proxy /api al backend
├── package.json
├── vite.config.js
├── tailwind.config.js
├── postcss.config.js
├── index.html
├── .oxlintrc.json              # Config de Oxlint
├── public/                     # Assets estáticos (favicon)
├── node_modules/
└── src/
    ├── main.jsx                # Punto de entrada: AuthProvider + Router
    ├── App.jsx                 # Definición de rutas
    ├── index.css               # Tailwind + clases base
    ├── api/
    │   └── client.js           # axios + interceptores + decodeJWT
    ├── contexts/
    │   └── AuthContext.jsx     # Estado global de autenticación
    ├── hooks/
    │   └── useBalance.js       # Hook centralizado para saldo
    ├── components/
    │   ├── AppLayout.jsx       # Wrapper con Header
    │   ├── Header.jsx          # Nav + user info + logout
    │   ├── BalanceCard.jsx     # Saldo + alias + copiar ID
    │   ├── ProtectedRoute.jsx  # Guard de rutas autenticadas
    │   ├── Chat.jsx            # Chat IA con confirmación
    │   └── TransactionForm.jsx # Form reusable para deposit/withdraw/transfer
    └── pages/
        ├── Login.jsx           # Form de login + credenciales demo
        ├── Register.jsx        # Form de registro
        ├── Dashboard.jsx       # Saldo + chat + últimas transacciones
        ├── Transactions.jsx    # 3 tabs de operaciones
        └── History.jsx         # Tabla paginada de transacciones
```

**Convenciones:**
- **Un archivo por componente** con el mismo nombre.
- **Subcarpetas por rol:** `api/`, `contexts/`, `hooks/`, `components/`, `pages/`.
- **Sin barrel exports** (no `index.js` en las carpetas). Import directo a cada archivo.

---

## 4. Decisiones arquitectónicas

### 4.1 Dos capas de estado

```
┌─────────────────────────────────────────────────────────┐
│  AuthContext (global)                                    │
│  ├── user (de /api/auth/me)                              │
│  ├── tbAccountID (del JWT decode)                        │
│  └── loading                                              │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│  useBalance() (hook local por página)                    │
│  ├── balance (de /api/account/balance)                   │
│  └── refresh() (para operaciones)                        │
└─────────────────────────────────────────────────────────┘
```

**Razón:**
- **AuthContext:** datos que no cambian durante la sesión (usuario, tb_account_id).
- **useBalance:** datos que cambian tras cada operación (saldo).

**Beneficio:** cada página decide cuándo consultar el saldo. No hay polling ni estado stale.

### 4.2 JWT como fuente del `tb_account_id`

**El `tb_account_id` no viene de `/api/auth/me`.** Viene del payload del JWT.

**Razón:**
- `/api/auth/me` devuelve datos de identidad (email, nombre, alias).
- El JWT ya contiene el `tb_account_id` como claim.
- **Evita un fetch extra** y mantiene la separación de responsabilidades.

**Decodificación:**

```js
// src/api/client.js
export function decodeJWT(token) {
  const base64Url = token.split('.')[1];
  const base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/');
  const padded = base64 + '='.repeat((4 - (base64.length % 4)) % 4);
  const json = decodeURIComponent(
    atob(padded)
      .split('')
      .map((c) => '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2))
      .join('')
  );
  return JSON.parse(json);
}
```

**Importante:** este decode **no verifica la firma**. Solo se usa para leer `tb_account_id` como dato auxiliar. Nunca para autorizar operaciones.

### 4.3 Balance centralizado con `useBalance()`

**Antes:** cada componente llamaba a `/api/account/balance` por su cuenta.

**Ahora:** un hook centraliza el acceso:

```js
const { balance, loading, error, refresh } = useBalance();
```

**Después de cualquier operación exitosa:**

```js
await refresh();
```

**Ejemplo en el Chat:**

```jsx
<Chat onBalanceChange={refreshBalance} />
```

Cuando el usuario confirma una operación en el chat, el saldo se actualiza automáticamente en el Dashboard.

### 4.4 Carga secuencial en el Dashboard

**Problema:** el Dashboard carga saldo + últimas 5 transacciones en paralelo. Bajo carga, TigerBeetle encola las operaciones y puede superar el timeout de 5s.

**Solución:** cargar en serie.

```jsx
useEffect(() => {
  async function load() {
    await refreshBalance();          // primero el saldo
    const { data } = await api.get('/api/transactions/history', {...});
    setRecentTxs(data.transactions); // después las transacciones
  }
  load();
}, []);
```

**Resultado:** sin timeouts, sin 500.

### 4.5 Confirmación en el chat: el flujo más complejo

**El chat es la pieza más compleja** porque maneja operaciones financieras en dos fases.

**Flujo:**

```
1. Usuario escribe "Deposita $50"
       ↓
2. POST /api/chat con el mensaje
       ↓
3. Backend responde con pending_confirmation:
   {
     reply: "Voy a depositar $50.00. ¿Confirmas?",
     pending_confirmation: {
       confirmation_token: "abc-123",
       type: "deposit",
       amount_cents: 5000
     }
   }
       ↓
4. Chat guarda pendingConfirmation en estado
       ↓
5. Se muestra un banner amarillo con botones:
   [Confirmar]  [Cancelar]
       ↓
6. Si el usuario confirma:
   POST /api/chat/confirm con el token
       ↓
7. Backend ejecuta la operación
       ↓
8. Se refresca el saldo vía onBalanceChange()
```

**Puntos clave del `Chat.jsx`:**

| Aspecto | Implementación |
|---|---|
| Input del usuario | **Deshabilitado** mientras hay `pendingConfirm` |
| Botón "Confirmar" | Llama a `confirmOperation()`, no a `sendMessage()` |
| Botón "Cancelar" | Limpia `pendingConfirm` **sin llamar al backend** |
| Refresco de saldo | `onBalanceChange` callback del padre |
| Sugerencias iniciales | 3 chips: "¿Cuánto tengo?", "Últimas 5", "Deposita $50" |
| Indicador de "escribiendo" | 3 puntos animados con `animate-bounce` |

**Por qué el input se deshabilita:** si el usuario escribe "no quiero" mientras hay `pendingConfirm`, no debe enviarse al chat. Solo puede confirmar o cancelar.

---

## 5. Componentes en detalle

### 5.1 `AuthContext.jsx`

**Responsabilidad:** mantener el estado de autenticación.

**Estado:**

| Campo | Tipo | Origen |
|---|---|---|
| `user` | `{id, email, full_name, alias, status}` | `/api/auth/me` |
| `tbAccountID` | `string` (hex) | JWT decode |
| `loading` | `bool` | Durante la restauración inicial |

**Acciones:**

| Acción | Qué hace |
|---|---|
| `login(email, password)` | POST `/api/auth/login`, guarda token, setea user |
| `register(email, password, fullName)` | POST `/api/auth/register` |
| `logout()` | Limpia localStorage y estado (stateless) |

**Al montar:**
1. Lee el token de `localStorage`.
2. Si existe, decodifica el JWT para obtener `tb_account_id`.
3. Llama a `/api/auth/me` para validar el token y obtener el `user`.
4. Si falla (401), limpia el token.

**No persiste `user` en localStorage.** Solo el token. El `user` se restaura siempre desde el backend.

### 5.2 `ProtectedRoute.jsx`

**Responsabilidad:** redirigir a `/login` si no hay sesión.

**Lógica:**

```jsx
if (loading) return <Spinner />;
if (!user) return <Navigate to="/login" state={{ from: location }} />;
return children;
```

**Puntos clave:**
- **Espera a `loading`.** Sin esto, redirige a `/login` antes de que el `AuthContext` termine de restaurar.
- **Guarda `location`** en el state para redirigir a la página original después del login.

### 5.3 `Header.jsx`

**Responsabilidad:** navegación superior.

**Elementos:**
- Logo + nombre del sistema.
- Nav: Dashboard, Transacciones, Historial.
- Nombre del usuario + alias.
- Botón "Salir".

**Responsive:**
- Desktop: nav en la misma fila.
- Mobile: nav en una segunda fila scrollable.

**Logout:**

```js
function handleLogout() {
  logout();                          // limpia el AuthContext
  navigate('/login', { replace: true }); // redirige
}
```

### 5.4 `BalanceCard.jsx`

**Responsabilidad:** mostrar el saldo y el alias.

**Props:**

| Prop | Tipo | Descripción |
|---|---|---|
| `balanceCents` | `int64 \| null` | Saldo en centavos |
| `alias` | `string` | Alias del usuario (`demo`) |
| `tbAccountID` | `string` | ID de la cuenta (hex 32) |

**Funcionalidad:**
- Formatea el saldo: `$1,065.00`.
- Muestra el alias: `@demo`.
- Botón "Copiar ID" que usa `navigator.clipboard.writeText`.

**Formato del saldo:**

```js
function formatCents(cents) {
  return `$${(cents / 100).toFixed(2)}`;
}
```

### 5.5 `AppLayout.jsx`

**Responsabilidad:** envolver páginas autenticadas con el Header.

**Estructura:**

```jsx
<div className="min-h-screen flex flex-col">
  <Header />
  <main className="flex-1 max-w-6xl mx-auto px-4 py-8">
    {children}
  </main>
</div>
```

**Uso:**

```jsx
<AppLayout>
  <h1>Dashboard</h1>
  {/* ... */}
</AppLayout>
```

**Beneficio:** todas las páginas autenticadas tienen el mismo layout sin repetir código.

### 5.6 `Chat.jsx`

**Responsabilidad:** chat con IA que ejecuta operaciones financieras.

**Props:**

| Prop | Tipo | Descripción |
|---|---|---|
| `onBalanceChange` | `() => Promise<void>` | Callback después de confirmar una operación |

**Estado:**

| Campo | Tipo |
|---|---|
| `messages` | `[{role, content}]` |
| `input` | `string` |
| `loading` | `bool` |
| `pendingConfirm` | `PendingOperation \| null` |
| `error` | `string \| null` |

**Funciones:**

| Función | Descripción |
|---|---|
| `sendMessage(text)` | POST `/api/chat`, agrega la respuesta a messages |
| `confirmOperation()` | POST `/api/chat/confirm`, ejecuta la operación pendiente |
| `cancelOperation()` | Limpia `pendingConfirm` sin llamar al backend |
| `handleSubmit(e)` | Wrapper para `sendMessage(input)` |

**Manejo de `pending_confirmation`:**

```js
if (data.pending_confirmation) {
  setPendingConfirm(data.pending_confirmation);
}
```

**Detección de tipo de operación en el banner:**

```jsx
{pendingConfirm.type === 'deposit' && 'Depósito'}
{pendingConfirm.type === 'withdraw' && 'Retiro'}
{pendingConfirm.type === 'transfer' && 'Transferencia'}
{' de '}
${(pendingConfirm.amount_cents / 100).toFixed(2)}
{pendingConfirm.type === 'transfer' && pendingConfirm.to_account_id && (
  <> a la cuenta {pendingConfirm.to_account_id.slice(0, 8)}…</>
)}
```

**Importante:** `to_account_id` es `""` para deposit/withdraw. Solo se muestra si `type === 'transfer'`.

### 5.7 `TransactionForm.jsx`

**Responsabilidad:** formulario reusable para las 3 operaciones.

**Props:**

| Prop | Tipo | Descripción |
|---|---|---|
| `type` | `'deposit' \| 'withdraw' \| 'transfer'` | Tipo de operación |
| `onSuccess` | `() => Promise<void>` | Callback después de una operación exitosa |

**Estado:**

| Campo | Tipo |
|---|---|
| `amount` | `string` (USD) |
| `toQuery` | `string` (email o alias) |
| `lookupResult` | `UserLookup \| null` |
| `loading` | `bool` |
| `error` / `success` | `string \| null` |

**Funciones:**

| Función | Descripción |
|---|---|
| `handleLookup(e)` | GET `/api/users/lookup` para buscar destinatario (solo transfer) |
| `handleSubmit(e)` | Convierte USD a centavos, envía la operación con Idempotency-Key |
| `resetForm()` | Limpia el estado después de una operación exitosa |

**Conversión USD → centavos:**

```js
const amountCents = Math.round(parseFloat(amount) * 100);
```

**Idempotency-Key:**

```js
const idempotencyKey = crypto.randomUUID();
await api.post(endpoint, body, {
  headers: { 'Idempotency-Key': idempotencyKey },
});
```

**Manejo de errores específicos:**

```js
if (code === 'INSUFFICIENT_FUNDS') setError('Saldo insuficiente para esta operación');
else if (code === 'DEST_NOT_FOUND') setError('La cuenta destino no existe');
else if (code === 'SAME_ACCOUNT') setError('No puedes transferir a tu propia cuenta');
else setError(message);
```

---

## 6. Páginas en detalle

### 6.1 `Login.jsx`

**Formulario:**
- Email (input type email).
- Password (input type password).

**Botones:**
- "Ingresar".
- "Usar credenciales demo →" que rellena el form con `demo@banco.com` / `Demo1234!`.

**Card de credenciales demo** debajo del form con las dos cuentas de prueba.

**Manejo de errores:**

| Código | Mensaje |
|---|---|
| `INVALID_CREDENTIALS` | "Credenciales inválidas" |
| `ACCOUNT_PENDING` | "Tu cuenta está siendo procesada. Intenta en unos minutos." |
| Otros | `message` del backend |

**Redirect:**
- Después del login exitoso, redirige a `from` (si vino de una ruta protegida) o a `/dashboard`.

### 6.2 `Register.jsx`

**Formulario:**
- Nombre completo.
- Email.
- Password (min 8 chars).

**Después del registro:**
- Muestra un banner verde: "✓ Cuenta creada. Redirigiendo al login..."
- Espera 1.5s.
- Redirige a `/login`.

**No auto-loguea.** El backend no devuelve un token en register. Es una decisión de diseño: separa register de login.

### 6.3 `Dashboard.jsx`

**Estructura:**

```
┌─────────────────────────────────────────────────────────┐
│  Header                                                  │
├─────────────────────────────────────────────────────────┤
│  Hola, [nombre] 👋                                       │
│                                                          │
│  ┌──────────────┐  ┌─────────────────────────────────┐  │
│  │ BalanceCard  │  │ Chat con IA                     │  │
│  │              │  │                                  │  │
│  │  $1,065.00   │  │  [mensajes]                      │  │
│  │  @demo       │  │                                  │  │
│  │  [Copiar ID] │  │  [input]                         │  │
│  └──────────────┘  └─────────────────────────────────┘  │
│                                                          │
│  ┌──────────────┐  ┌─────────────────────────────────┐  │
│  │ Cargar       │  │ Últimas transacciones            │  │
│  │ $1,000 demo  │  │                                  │  │
│  │              │  │  Depósito    +$50.00   15 sep    │  │
│  │ Ver transac. │  │  Retiro      −$30.00   15 sep    │  │
│  └──────────────┘  │  Ver todas →                     │  │
│                    └─────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

**Cargas:**
- Saldo (vía `useBalance`).
- Últimas 5 transacciones (vía `GET /api/transactions/history?limit=5`).
- **En serie**, no en paralelo, para evitar timeouts de TigerBeetle.

**Botón "Cargar $1,000 de demo":**
- POST `/api/transactions/demo-topup`.
- Después de éxito: refresca saldo + transacciones.

**Chat integrado:**
- Se pasa `refreshBalance` como `onBalanceChange`.
- Cuando el usuario confirma una operación en el chat, el saldo se actualiza automáticamente.

### 6.4 `Transactions.jsx`

**Estructura:**

```
┌─────────────────────────────────────────────────────────┐
│  Transacciones                                           │
│                                                          │
│  ┌──────────────┐  ┌─────────────────────────────────┐  │
│  │ BalanceCard  │  │ [Depositar] [Retirar] [Transferir]│ │
│  │              │  ├─────────────────────────────────┤  │
│  │  $1,065.00   │  │                                  │  │
│  │  @demo       │  │  Form según tab                  │  │
│  │              │  │                                  │  │
│  └──────────────┘  │  [Submit]                        │  │
│                    └─────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

**3 tabs:**
- **Depositar** (icono `ArrowDownToLine`).
- **Retirar** (icono `ArrowUpFromLine`).
- **Transferir** (icono `Send`).

**`key={activeTab}` en el form:**
- Fuerza el remount al cambiar de tab.
- Limpia el estado (monto, búsqueda, errores).

**Formulario de transferencia:**
- Input de email/alias + botón "Buscar".
- Cuando encuentra al usuario, muestra sus datos con ✓ y un botón X para cambiar.
- Después de buscar, el monto.

**Botón de submit:**
- Verde para depositar.
- Rojo para retirar.
- Azul para transferir.

### 6.5 `History.jsx`

**Estructura:**

```
┌─────────────────────────────────────────────────────────┐
│  Historial                                               │
│  N transacciones en total                                │
│                                                          │
│  ┌─────────────────────────────────────────────────┐    │
│  │ Tipo          │ Fecha              │ Monto       │    │
│  ├─────────────────────────────────────────────────┤    │
│  │ ↓ Depósito    │ 15 sep, 18:35      │ +$50.00     │    │
│  │ ↑ Retiro      │ 15 sep, 18:30      │ −$30.00     │    │
│  │ ↓ Depósito    │ 15 sep, 18:00      │ +$1,000.00  │    │
│  └─────────────────────────────────────────────────┘    │
│                                                          │
│  [← Anterior]    Página 1 de 3    [Siguiente →]         │
└─────────────────────────────────────────────────────────┘
```

**Cálculo de dirección:**

```js
function computeDirection(tx) {
  if (tx.credit_account_id === tbAccountID) return 'in';
  if (tx.debit_account_id === tbAccountID) return 'out';
  return 'unknown';
}
```

**Visualización:**
- **`in`:** ícono `ArrowDownLeft` verde, monto `+$X`.
- **`out`:** ícono `ArrowUpRight` rojo, monto `−$X`.

**Paginación:**
- 10 items por página.
- Botones "Anterior" / "Siguiente" deshabilitados en los extremos.
- `page` es estado local. Cambiar `page` dispara `useEffect`.

**3 estados:**
- **Cargando:** card con "Cargando...".
- **Vacío:** ícono `Inbox` + "No tienes transacciones todavía".
- **Con datos:** tabla.

---

## 7. Flujos de usuario

### 7.1 Login

```
Usuario → Login.jsx
   ↓
Form: email + password
   ↓
AuthContext.login(email, password)
   ↓
POST /api/auth/login
   ↓
Guardar token en localStorage
   ↓
Decodificar JWT → tbAccountID
   ↓
setUser + setTbAccountID
   ↓
navigate('/dashboard')
```

### 7.2 Restauración de sesión al recargar

```
main.jsx → AuthProvider monta
   ↓
localStorage.getItem('token')
   ↓
¿Existe?
   ├── No → setLoading(false)
   └── Sí:
       ├── decodeJWT(token) → tbAccountID
       ├── GET /api/auth/me
       │   ├── 200 → setUser
       │   └── 401 → limpiar token
       └── setLoading(false)
   ↓
ProtectedRoute verifica user
```

### 7.3 Operación por chat

```
Usuario escribe "Deposita $50"
   ↓
POST /api/chat
   ↓
Backend responde con pending_confirmation
   ↓
Chat.jsx setPendingConfirm(...)
   ↓
Banner amarillo con [Confirmar] [Cancelar]
   ↓
¿Confirmar?
   ├── Sí → POST /api/chat/confirm
   │        → onBalanceChange()
   │        → saldo actualizado
   └── No → setPendingConfirm(null)
```

### 7.4 Transferencia con búsqueda

```
Usuario → Transactions → Tab "Transferir"
   ↓
Input: "demo2@banco.com"
   ↓
Click "Buscar"
   ↓
GET /api/users/lookup?email=demo2@banco.com
   ↓
lookupResult: {full_name, alias, tb_account_id}
   ↓
Mostrar: "✓ Usuario Demo 2 · @demo2 · demo2@banco.com"
   ↓
Input monto: "15"
   ↓
Click "Transferir"
   ↓
POST /api/transactions/transfer con Idempotency-Key
   ↓
onSuccess() → refreshBalance()
   ↓
Saldo actualizado
```

---

## 8. API client — cómo funciona

### 8.1 Interceptor de request

```js
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});
```

**Qué hace:** agrega el JWT a cada request automáticamente. Sin esto, habría que pasar el token manualmente en cada llamada.

### 8.2 Interceptor de response

```js
api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token');
      if (window.location.pathname !== '/login') {
        window.location.href = '/login';
      }
    }
    return Promise.reject(error);
  }
);
```

**Qué hace:** si el backend devuelve 401 (token expirado), limpia el token y redirige a login.

**Limitación conocida:** si varias requests concurrentes reciben 401, se disparan varios redirects. Para esta prueba es aceptable. Una versión más robusta delegaría el manejo al `AuthContext`.

### 8.3 Proxy en desarrollo

**En `vite.config.js`:**

```js
server: {
  proxy: {
    '/api': 'http://localhost:8080',
  },
},
```

**Qué hace:** las requests a `/api/*` en el dev server se redirigen al backend en `localhost:8080`. **Evita CORS.**

### 8.4 Proxy en producción

**En `nginx.conf`:**

```nginx
location /api/ {
  proxy_pass http://backend:8080/api/;
  proxy_read_timeout 120s;
}
```

**Qué hace:** lo mismo que el proxy de Vite, pero en nginx. Usa el **hostname del servicio** (resuelto por Docker DNS).

**`proxy_read_timeout 120s`:** el chat con LLM puede tardar. Sin esto, nginx corta a los 60s.

---

## 9. Docker

### 9.1 Dockerfile

```dockerfile
# --- Build stage ---
FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

# --- Runtime stage ---
FROM nginx:alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
```

**Decisión multi-stage:**
- **Builder:** Node 20 con `npm ci` (reproducible).
- **Runtime:** nginx alpine con solo `dist/` y `nginx.conf`.
- **Tamaño final:** ~30-40 MB.

### 9.2 docker-compose.yml

```yaml
frontend:
  build:
    context: ./frontend
    dockerfile: Dockerfile
  container_name: banking-frontend
  depends_on:
    - backend
  ports:
    - "80:80"
  networks:
    - banking_net
```

**`depends_on: [backend]`:** el frontend espera a que el backend esté arrancado. No usa `condition: service_healthy` porque el backend no tiene healthcheck propio.

### 9.3 Cómo levantar

```bash
cd ~/proyectos/banking-system
docker compose up -d --build
sleep 30
docker ps --filter name=banking-
```

**Esperado:**

```
banking-frontend     Up X seconds    0.0.0.0:80->80/tcp
banking-backend      Up X seconds    0.0.0.0:8080->8080/tcp
banking-postgres     Up X seconds (healthy)
banking-tigerbeetle  Up X seconds (healthy)
```

**Acceso:**
- Frontend: `http://localhost`
- Backend: `http://localhost:8080`
- Postgres: `localhost:5432`
- TigerBeetle: `localhost:3000`

---

## 10. Convenciones y patrones

### 10.1 Nombres

| Tipo | Convención | Ejemplo |
|---|---|---|
| Componente | PascalCase | `Chat.jsx`, `BalanceCard.jsx` |
| Hook | camelCase con `use` | `useBalance.js` |
| Utilidad | camelCase | `client.js` |
| Contexto | PascalCase | `AuthContext.jsx` |
| Página | PascalCase | `Dashboard.jsx` |

### 10.2 Estilos Tailwind

**Clases semánticas en `index.css`:**

```css
.btn-primary   { @apply btn bg-brand-600 text-white hover:bg-brand-700; }
.card          { @apply bg-white rounded-xl shadow-sm border border-gray-200; }
.input         { @apply w-full px-3 py-2 border border-gray-300 rounded-lg ...; }
.error-box     { @apply bg-red-50 border border-red-200 text-red-700 ...; }
.success-box   { @apply bg-green-50 border border-green-200 text-green-700 ...; }
```

**Razón:** evita repetir 10 clases en cada botón. Los componentes usan `<button className="btn-primary">`.

### 10.3 Manejo de errores

**Siempre mostrar `message` del backend, no `code`.**

```js
const { code, message } = err.response?.data || {};
setError(message || 'Error genérico');
```

**Excepción:** algunos códigos se mapean a mensajes más claros en el frontend:

```js
if (code === 'INSUFFICIENT_FUNDS') setError('Saldo insuficiente');
```

### 10.4 Formato de montos

**Siempre en centavos en el backend. Conversión en el frontend:**

```js
const display = `$${(cents / 100).toFixed(2)}`;
const inputToCents = (usd) => Math.round(parseFloat(usd) * 100);
```

**Nunca usar `float64` para manipular dinero.** Solo en el input y en el display.

### 10.5 Formato de fechas

```js
new Date(iso).toLocaleString('es-ES', {
  day: '2-digit',
  month: 'short',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
});
// → "15 sep 2026, 18:35"
```

---

## 11. Lo que falta

| Feature | Prioridad | Duración |
|---|---|---|
| **Modal de comprobante** después de cada operación | Media | 30 min |
| **Gráficas** con recharts en Dashboard | Baja | 30 min |
| **Rate limiting** visible en login | Baja | 5 min |
| **Modo oscuro** | Baja | 1h |
| **Notificaciones** push/in-app | Baja | 2h |

**Ninguna de estas bloquea la entrega.**

---

## 12. Comandos útiles

### 12.1 Desarrollo

```bash
cd frontend
npm run dev        # Dev server con hot reload en :5173
npm run build      # Build de producción
npm run lint       # Oxlint
npm run preview    # Preview del build
```

### 12.2 Docker

```bash
cd ..
docker compose up -d --build
docker compose logs -f frontend
docker compose down
docker compose down -v  # borra volúmenes
```

### 12.3 Verificar desde la Mac

```bash
# Ver el frontend (si está corriendo en la VM)
curl -I http://192.168.68.57/
# → HTTP/1.1 200 OK
```

---

## 13. Estado final

**El frontend está completo y funcional.**

| Batch | Contenido | Estado |
|---|---|---|
| 1 | Setup + Auth + Router | ✅ |
| 2 | Header + BalanceCard + Layout | ✅ |
| 3 | Login + Register | ✅ |
| 4a | Dashboard real + demo-topup | ✅ |
| 4b | Chat con confirmación | ✅ |
| 5a | Transactions + búsqueda | ✅ |
| 5b | History + paginación | ✅ |
| 6 | Docker + nginx | ✅ |

**Lo que funciona end-to-end:**
- Registro, login, logout.
- Saldo, alias, copiar ID.
- Cargar $1,000 de demo.
- Chat con IA que ejecuta operaciones.
- Confirmación en dos fases en el chat.
- 3 formularios de transacciones con idempotencia.
- Búsqueda de destinatario por email/alias.
- Historial paginado con direcciones.
- Docker Compose levanta los 4 servicios.

---

**Documento generado:** 15 de septiembre de 2026
**Versión:** 1.0
**Mantenedor:** Gean Munoz