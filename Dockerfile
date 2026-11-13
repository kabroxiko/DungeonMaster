# syntax=docker/dockerfile:1.4
# Production image: the Vue client is built by Vue CLI, the Go API is built with the module's toolchain, and nginx
# serves the SPA while proxying /api to gmai-server on localhost.
#
# VUE_APP_DM_GOOGLE_CLIENT_ID is compiled into the SPA, so it is a build argument rather than a runtime variable.

########################
# Frontend build stage
########################
FROM node:22-alpine AS frontend-builder
WORKDIR /src/client/dungeonmaster

ARG VUE_APP_DM_GOOGLE_CLIENT_ID=""
ENV VUE_APP_DM_GOOGLE_CLIENT_ID=${VUE_APP_DM_GOOGLE_CLIENT_ID}
# Vue CLI/webpack can exceed the default heap while minifying.
ENV NODE_OPTIONS=--max-old-space-size=4096

COPY client/dungeonmaster/package.json client/dungeonmaster/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --include=dev --silent

COPY client/dungeonmaster/ ./
RUN npm run build

########################
# Backend build stage
########################
FROM golang:1.25-alpine AS backend-builder
WORKDIR /src/go

COPY go/go.mod go/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY go/ ./
RUN --mount=type=cache,target=/root/.cache/go-build \
	CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/gmai-server ./cmd/gmai-server

########################
# Runtime image (nginx + gmai-server)
########################
FROM nginx:stable-alpine AS runtime

# Run both processes as an unprivileged user: nginx listens on 8080 and writes only to /tmp.
RUN addgroup -S app && adduser -S -G app app

COPY --from=backend-builder /out/gmai-server /app/gmai-server
COPY --from=frontend-builder /src/client/dungeonmaster/dist /app/static
COPY assets/world-settings.json /app/assets/world-settings.json
COPY docker/nginx.conf /etc/nginx/nginx.conf
COPY docker/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN chmod +x /app/gmai-server /usr/local/bin/docker-entrypoint.sh \
	&& mkdir -p /tmp/nginx \
	&& chown -R app:app /app /tmp/nginx

WORKDIR /app
USER app

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
	CMD wget -qO- http://127.0.0.1:8080/api/meta >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
