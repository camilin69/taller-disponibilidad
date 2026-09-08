# --- Etapa 1: construccion del panel React ---
FROM node:20-alpine AS constructor
WORKDIR /app
COPY package.json ./
RUN npm install
COPY tsconfig.json vite.config.ts index.html ./
COPY public/ ./public/
COPY src/ ./src/
RUN npm run build

# --- Etapa 2: servidor estatico ---
FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=constructor /app/dist /usr/share/nginx/html
# El script se ejecuta al arrancar el contenedor (hook de la imagen nginx) y
# escribe config.js con la URL del dispatcher tomada del entorno.
COPY docker-entrypoint.d/40-config.sh /docker-entrypoint.d/40-config.sh
RUN chmod +x /docker-entrypoint.d/40-config.sh
ENV DISPATCHER_URL=http://localhost:8080
EXPOSE 80
