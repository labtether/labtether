# ──────────────────────────────────────────────────────────────
# LabTether All-in-One Image
# Bundles: Hub API (Go) · Next.js Console · PostgreSQL 18
# Process supervisor: s6-overlay v3
# Remote desktop (guacd) runs as a separate container via compose.
# ──────────────────────────────────────────────────────────────

# ── Stage 1: Go builder ──────────────────────────────────────
FROM golang:1.27.2-alpine3.24@sha256:85dc1069ac644ea3c527b177303a406eb3358192816cd7f9e5848eb658851673 AS go-builder

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
FROM node:26.11.1-alpine3.24@sha256:143494b1da2945f061539253adc65e4f1569ddf07da2d384c022c791a9d90a4a AS node-toolchain
RUN npm install --global npm@12.2.0

FROM node-toolchain AS node-deps

WORKDIR /app
COPY web/console/package.json web/console/package-lock.json web/console/.npmrc ./
COPY web/console/scripts/ ./scripts/
RUN npm ci

# ── Stage 3: Next.js build ───────────────────────────────────
FROM node-toolchain AS node-builder

WORKDIR /app
COPY --from=node-deps /app/node_modules ./node_modules
COPY web/console/ .

ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

# ── Stage 4: Runtime ─────────────────────────────────────────
FROM node:26.11.1-alpine3.24@sha256:143494b1da2945f061539253adc65e4f1569ddf07da2d384c022c791a9d90a4a
SHELL ["/bin/ash", "-eo", "pipefail", "-c"]

ARG S6_OVERLAY_VERSION=3.2.3.2
ARG TARGETARCH
ARG S6_OVERLAY_NOARCH_SHA256=5379750ed30a84bbd2e2dd74847ba6b5bd29cd0b2e3ea2ec58049b57eb2eda12
ARG S6_OVERLAY_X86_64_SHA256=e6befcc96a437a3831386ecfc51808c5d3e939dc5fe3c02ae9284599e8aa2408
ARG S6_OVERLAY_AARCH64_SHA256=b17f17a82e7a515c682a91edaf2ffdabb73f891981b6c1fd712115693a2f8b4c

# Install s6-overlay (arch-aware)
RUN set -eux; \
    wget -q "https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-noarch.tar.xz" -O /tmp/s6-overlay-noarch.tar.xz; \
    echo "${S6_OVERLAY_NOARCH_SHA256}  /tmp/s6-overlay-noarch.tar.xz" | sha256sum -c -; \
    tar -C / -Jxpf /tmp/s6-overlay-noarch.tar.xz; \
    rm /tmp/s6-overlay-noarch.tar.xz; \
    case "${TARGETARCH}" in \
      amd64) S6_ARCH="x86_64"; S6_SHA256="${S6_OVERLAY_X86_64_SHA256}" ;; \
      arm64) S6_ARCH="aarch64"; S6_SHA256="${S6_OVERLAY_AARCH64_SHA256}" ;; \
      *)     echo "Unsupported arch: ${TARGETARCH}" && exit 1 ;; \
    esac; \
    wget -q "https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-${S6_ARCH}.tar.xz" -O /tmp/s6-overlay-arch.tar.xz; \
    echo "${S6_SHA256}  /tmp/s6-overlay-arch.tar.xz" | sha256sum -c -; \
    tar -C / -Jxpf /tmp/s6-overlay-arch.tar.xz; \
    rm /tmp/s6-overlay-arch.tar.xz

# Alpine's stable branch keeps these package families compatible while its
# repositories replace old patch releases with security updates. Exact apk
# patch pins make a clean rebuild fail once the old packages are retired.
RUN apk add --no-cache \
    bash \
    ca-certificates \
    postgresql18 \
    postgresql18-contrib && \
    node -e "if (process.versions.node !== '26.11.1') process.exit(1)" && \
    psql --version | grep -Fq 'PostgreSQL) 18.' && \
    adduser -D -u 65532 labtether && \
    rm -rf /usr/local/lib/node_modules/npm /opt/yarn-v* && \
    rm -f /usr/local/bin/npm /usr/local/bin/npx /usr/local/bin/yarn /usr/local/bin/yarnpkg

# ── Copy artefacts from build stages ─────────────────────────

# Go binary
COPY --from=go-builder /out/labtether /usr/local/bin/labtether

# Next.js standalone + static assets
COPY --from=node-builder --chown=labtether:labtether /app/.next/standalone /app/console/
COPY --from=node-builder --chown=labtether:labtether /app/.next/static     /app/console/.next/static
COPY --from=node-builder --chown=labtether:labtether /app/public            /app/console/public
COPY --from=node-builder --chown=labtether:labtether /app/scripts/console-runtime.mjs /app/scripts/runtime-proxy.mjs /app/scripts/console-ingress.mjs /app/console/scripts/

# Agent distribution (populated by CI; .keep ensures layer exists)
COPY agent-dist/ /opt/labtether/agents/

# ── Data directories ─────────────────────────────────────────
RUN mkdir -p /data/postgres /data/certs /data/agents /data/config /data/install \
             /run/postgresql /run/labtether /ca && \
    chown -R postgres:postgres /data/postgres /run/postgresql && \
    chown -R labtether:labtether /data /run/labtether /ca && \
    chown -R postgres:postgres /data/postgres

# ── s6 service definitions ───────────────────────────────────
COPY build/s6/s6-rc.d /etc/s6-overlay/s6-rc.d
COPY build/s6/user     /etc/s6-overlay/s6-rc.d/user
COPY build/s6/scripts  /etc/s6-overlay/scripts
RUN find /etc/s6-overlay/s6-rc.d -name 'run' -exec chmod 755 {} + && \
    find /etc/s6-overlay/s6-rc.d -name 'up'  -exec chmod 755 {} + && \
    find /etc/s6-overlay/scripts -type f -exec chmod 755 {} +

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
    NEXT_TELEMETRY_DISABLED=1 \
    HOSTNAME=0.0.0.0

EXPOSE 3000 8080 8443

# Do not declare VOLUME /data here. This image also runs credential-free split
# services, and Docker allocates image-declared anonymous volumes before
# Compose can override them. Hub installation paths mount /data explicitly;
# the all-in-one s6 graph fails closed when that mount is absent.

# CMD (not ENTRYPOINT) so compose command: overrides bypass s6
CMD ["/init"]
