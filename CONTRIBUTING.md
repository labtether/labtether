# Contributing

Contributions to LabTether are welcome. This guide covers the practical workflow for reporting bugs, suggesting features, and submitting code.

## Reporting Bugs

1. Check [KNOWN_ISSUES.md](KNOWN_ISSUES.md) first.
2. Follow the diagnostic checklist in [SUPPORT.md](SUPPORT.md).
3. Open a GitHub Issue with version, deployment path, reproduction steps, and relevant logs (secrets redacted).

For security vulnerabilities, do **not** open a public issue. Follow [SECURITY.md](SECURITY.md).

## Suggesting Features

Use GitHub Discussions for feature ideas and design conversations. If Discussions are not enabled, open an issue and label it as a feature request.

## Development Setup

### Prerequisites

| Tool | Version |
|---|---|
| Go | 1.26+ |
| Node.js | LTS (for the Next.js console) |
| Docker + Compose | Recent stable |
| PostgreSQL | Managed via Docker Compose (no separate install needed) |

### First-Time Setup

The bootstrap command handles env configuration, database migration, Docker Compose startup, smoke tests, and a health check:

```bash
make bootstrap
```

### Manual Dev Workflow

Start the backend and frontend dev runtimes:

```bash
make dev-up
```

This runs the Go backend and Next.js dev server in background sessions. To restart cleanly:

```bash
make dev-up-restart
```

Stop everything:

```bash
make dev-stop-all
```

### Environment

Copy `.env.deploy.example` to `.env.deploy` and review the defaults. The dev backend reads from `.env` when present. Key variables:

- `DATABASE_URL` -- Postgres connection string (defaults to local Docker Postgres).
- `LABTETHER_ENCRYPTION_KEY` -- required; generate with `openssl rand -base64 32`.
- `LABTETHER_ADMIN_PASSWORD` -- set for auto-provisioned admin, or leave blank to use browser setup.

## Running Tests

Quick validation (Go vet + Go tests + TypeScript type check):

```bash
make check
```

Individual targets:

```bash
make fmt          # gofmt
make lint         # go vet
make test         # go test ./...
cd web/console && npx tsc --noEmit   # TypeScript
```

Integration and smoke tests against a running stack:

```bash
make smoke-test
make integration-test
```

Docs quality gates:

```bash
make check-docs
```

## Code Style And Conventions

- **Go**: standard `gofmt` formatting; `go vet` must pass with no warnings.
- **TypeScript/React**: follow existing App Router conventions and server/client boundaries in `web/console/`.
- Match the conventions of the subtree you are touching before introducing new patterns.
- Keep platform-specific behavior behind clear interfaces.
- Keep tests close to the code they cover.

## Pull Request Process

1. Review active priorities in `notes/TODO.md` and architectural decisions in `docs/internal/ADR.md`.
2. Create a feature branch from `main`.
3. Make your changes. Keep commits scoped and additive.
4. Run the full check suite:
   ```bash
   make check
   ```
5. Update documentation:
   - `docs/internal/` for architecture or config changes.
   - `docs/internal/ADR.md` for design decisions.
   - `notes/TODO.md` and `notes/PROGRESS_LOG.md` if applicable.
6. Open a pull request with a clear description of the change and its motivation.
7. CI must pass before merge.

If your change does not require docs updates, note the rationale in the PR description.

## Security Issues

Do not open a public issue for security vulnerabilities. Follow the private reporting process in [SECURITY.md](SECURITY.md).

## License

By contributing, you agree that your contributions are licensed under the [Apache License 2.0](LICENSE).
