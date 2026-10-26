FROM golang:1.27.3-alpine3.24@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS builder

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

FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a
RUN apk add --no-cache \
    bash=5.3.9-r1 \
    ca-certificates=20261020-r0 && \
    adduser -D -u 65532 labtether
COPY --from=builder /out/service /service
COPY --from=builder --chown=65532:65532 /out/data /data
COPY --from=builder --chown=65532:65532 /out/ca /ca
RUN mkdir -p /run/labtether && chown -R 65532:65532 /run/labtether
# Agent binaries and manifest (populated by CI before docker build)
COPY --chown=65532:65532 agent-dist/ /opt/labtether/agents/
USER 65532
ENTRYPOINT ["/service"]
