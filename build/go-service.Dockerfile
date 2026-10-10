FROM golang:1.27.2-alpine3.24@sha256:85dc1069ac644ea3c527b177303a406eb3358192816cd7f9e5848eb658851673 AS builder

ARG SERVICE_DIR
ARG APP_VERSION=dev
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 \
    GOOS=${TARGETOS:-$(go env GOOS)} \
    GOARCH=${TARGETARCH:-$(go env GOARCH)} \
    go build -trimpath -ldflags="-s -w -X main.version=${APP_VERSION}" -o /out/service ./${SERVICE_DIR}
RUN mkdir -p /out/data /out/ca

FROM alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6
RUN apk add --no-cache \
    bash=5.3.9-r1 \
    ca-certificates=20260909-r0 && \
    adduser -D -u 65532 labtether
COPY --from=builder /out/service /service
COPY --from=builder --chown=65532:65532 /out/data /data
COPY --from=builder --chown=65532:65532 /out/ca /ca
RUN mkdir -p /run/labtether && chown -R 65532:65532 /run/labtether
# Agent binaries and manifest (populated by CI before docker build)
COPY --chown=65532:65532 agent-dist/ /opt/labtether/agents/
USER 65532
ENTRYPOINT ["/service"]
