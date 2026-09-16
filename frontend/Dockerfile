# --- Build stage ---
FROM node:20-alpine AS builder

WORKDIR /app

# Copiar package files primero (mejor caching)
COPY package*.json ./
RUN npm ci

# Copiar el resto
COPY . .

# Build de producción
RUN npm run build

# --- Runtime stage ---
FROM nginx:alpine

# Config custom
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Copiar el build
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]