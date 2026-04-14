# ──────────────────────────────────────────────────────────────
# LabTether All-in-One Image
# Bundles: Hub API (Go) · Next.js Console · PostgreSQL 18 · guacd
# Process supervisor: s6-overlay v3
# ──────────────────────────────────────────────────────────────

# ── Stage 1: Go builder ──────────────────────────────────────
FROM golang:1.26-alpine AS go-builder

ARG APP_VERSION=dev
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    go build -trimpath \
      -ldflags="-s -w -X main.version=${APP_VERSION}" \
      -o /out/labtether ./cmd/labtether

# ── Stage 2: Node dependency install ─────────────────────────
FROM node:24-alpine AS node-deps

WORKDIR /app
COPY web/console/package.json web/console/package-lock.json ./
COPY web/console/scripts/ ./scripts/
RUN npm ci

# ── Stage 3: Next.js build ───────────────────────────────────
FROM node:24-alpine AS node-builder

WORKDIR /app
COPY --from=node-deps /app/node_modules ./node_modules
COPY web/console/ .

ENV LABTETHER_API_BASE_URL=http://localhost:8080
RUN npm run build

# ── Stage 4: guacd source (binary + libs) ────────────────────
FROM guacamole/guacd:1.6.0 AS guacd-source

# ── Stage 5: Runtime ─────────────────────────────────────────
FROM alpine:3.21

ARG S6_OVERLAY_VERSION=3.2.0.2
ARG TARGETARCH

# Install s6-overlay (arch-aware)
ADD https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-noarch.tar.xz /tmp
RUN tar -C / -Jxpf /tmp/s6-overlay-noarch.tar.xz && rm /tmp/s6-overlay-noarch.tar.xz

RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) S6_ARCH="x86_64" ;; \
      arm64) S6_ARCH="aarch64" ;; \
      *)     echo "Unsupported arch: ${TARGETARCH}" && exit 1 ;; \
    esac; \
    wget -q "https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-${S6_ARCH}.tar.xz" -O /tmp/s6-overlay-arch.tar.xz && \
    tar -C / -Jxpf /tmp/s6-overlay-arch.tar.xz && \
    rm /tmp/s6-overlay-arch.tar.xz

# Runtime dependencies
RUN apk add --no-cache \
    bash \
    ca-certificates \
    nodejs \
    postgresql18 \
    postgresql18-contrib \
    cairo \
    libjpeg-turbo \
    libpng \
    libwebp \
    pango \
    libssh2 \
    libvncserver \
    ossp-uuid \
    freerdp-libs \
    libtelnet

# Create users
RUN adduser -D -u 65532 labtether && \
    adduser -D -s /bin/sh postgres && \
    adduser -D -s /bin/sh guacd

# ── Copy artefacts from build stages ─────────────────────────

# Go binary
COPY --from=go-builder /out/labtether /usr/local/bin/labtether

# Next.js standalone + static assets
COPY --from=node-builder /app/.next/standalone /app/console/
COPY --from=node-builder /app/.next/static     /app/console/.next/static
COPY --from=node-builder /app/public            /app/console/public

# guacd binary + libraries (guacamole installs to /opt/guacamole/)
COPY --from=guacd-source /opt/guacamole/sbin/guacd       /opt/guacamole/sbin/guacd
COPY --from=guacd-source /opt/guacamole/lib/             /opt/guacamole/lib/
COPY --from=guacd-source /opt/guacamole/bin/             /opt/guacamole/bin/

# Agent distribution (populated by CI; .keep ensures layer exists)
COPY agent-dist/ /opt/labtether/agents/

# ── Data directories ─────────────────────────────────────────
RUN mkdir -p /data/postgres /data/certs /data/agents /data/config \
             /run/postgresql /run/labtether && \
    chown -R postgres:postgres /data/postgres /run/postgresql && \
    chown -R labtether:labtether /data/certs /data/agents /data/config /run/labtether

# ── s6 service definitions ───────────────────────────────────
COPY build/s6/s6-rc.d /etc/s6-overlay/s6-rc.d
COPY build/s6/user     /etc/s6-overlay/s6-rc.d/user
RUN find /etc/s6-overlay/s6-rc.d -name 'run' -exec chmod 755 {} + && \
    find /etc/s6-overlay/s6-rc.d -name 'up'  -exec chmod 755 {} +

# ── Environment ──────────────────────────────────────────────
ENV S6_KEEP_ENV=1 \
    S6_BEHAVIOUR_IF_STAGE2_FAILS=2 \
    POSTGRES_DB=labtether \
    POSTGRES_USER=labtether \
    LABTETHER_DATA_DIR=/data \
    LABTETHER_TLS_MODE=auto \
    API_PORT=8080 \
    LABTETHER_HTTPS_PORT=8443 \
    PORT=3000 \
    LABTETHER_API_BASE_URL=http://localhost:8080 \
    LD_LIBRARY_PATH=/opt/guacamole/lib

EXPOSE 3000 8080 8443
VOLUME /data

# CMD (not ENTRYPOINT) so compose command: overrides bypass s6
CMD ["/init"]
