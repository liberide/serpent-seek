# -- web build stage ---------------------------------------------------------
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ .
RUN npm run build

# -- backend build stage -----------------------------------------------------
FROM golang:1.26-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/build ./web/build
ARG TAG=1.0.0
ARG COMMIT=dev
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X github.com/liberide/serpent-seek/internal/version.Version=${TAG} -X github.com/liberide/serpent-seek/internal/version.Commit=${COMMIT} -X github.com/liberide/serpent-seek/internal/version.BuildDate=${BUILD_DATE}" \
    -o /out/serpentseek ./cmd/serpentseek

# -- final stage -------------------------------------------------------------
FROM alpine:3.21
ARG TAG=1.0.0
ARG COMMIT=dev
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="SerpentSeek" \
      org.opencontainers.image.description="Search gateway microservice for Open WebUI" \
      org.opencontainers.image.version="${TAG}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.source="https://github.com/liberide/serpent-seek" \
      org.opencontainers.image.licenses="GPL-3.0-only"
RUN addgroup -S serpent && adduser -S serpent -G serpent
RUN apk add --no-cache wget
WORKDIR /app
COPY --from=backend /out/serpentseek /app/serpentseek
# Ship the legal notices with the image: the binary statically links Go
# modules and embeds the web bundle, so their license texts must accompany it.
COPY LICENSE NOTICE THIRD_PARTY_LICENSES.md /app/licenses/
# Create the data dir owned by the runtime user so the process (and a named
# volume initialized from the image) can write the SQLite database.
RUN mkdir -p /app/data && chown -R serpent:serpent /app/data
VOLUME ["/app/data"]
USER serpent
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/app/serpentseek"]
