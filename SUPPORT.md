# Support

This page explains how to get help with LabTether and what to include when reporting a problem.

## Before You Open A Report

1. Read the root [README](README.md) and the [Documentation](https://labtether.com/docs).
2. Check [KNOWN_ISSUES.md](KNOWN_ISSUES.md) to see whether the behavior is already documented.
3. Run the relevant troubleshooting guide from the wiki:
   - [Quick Diagnostics](https://labtether.com/docs/wiki/troubleshooting/quick-diagnostics)
   - [Desktop Connection Failures](https://labtether.com/docs/wiki/troubleshooting/desktop-connection-failures)
   - [Auth and Login Issues](https://labtether.com/docs/wiki/troubleshooting/auth-login-issues)
4. If this is a security issue, stop here and follow [SECURITY.md](SECURITY.md) instead of filing a public report.

## Best Support Path

- **Bug reports**: use the project's public issue tracker.
- **Usage questions**: use project discussions if enabled; otherwise open an issue and label it clearly as support/question traffic.
- **Security issues**: do not open a public issue; follow [SECURITY.md](SECURITY.md).

## Include This In Your Report

- LabTether version, commit, or branch.
- Deployment path:
  - Docker Compose
  - Home Assistant custom integration
  - Home Assistant add-on
  - iOS companion
  - macOS menu bar agent
- Host OS and architecture.
- Browser or mobile OS version when relevant.
- Exact steps to reproduce.
- Expected behavior and actual behavior.
- Relevant logs, screenshots, or screen recordings with secrets redacted.

## Helpful Diagnostics To Attach

- `make setup-doctor`
- `docker compose ps`
- `docker compose logs --tail=200`
- `make smoke-test`
- `./scripts/desktop-smoke-test.sh --list-targets`
- For native companion app diagnostics, see each app's private repo.

If the problem is connector-specific, include the affected connector type and whether the failure happens during test, sync, or runtime use.

## Redact Before Sharing

- API tokens
- enrollment tokens
- passwords
- private keys
- raw CA key material
- full bearer tokens or cookies

When possible, prefer status summaries and error messages over raw secret-bearing payload dumps.

## Useful Reference Docs

- [Documentation](https://labtether.com/docs)
- [Privacy Policy](PRIVACY.md)
- [User Guide](https://labtether.com/docs)
- [Supported Release Matrix](https://labtether.com/docs/wiki/reference/supported-release-matrix)
- [Production Deployment Checklist](https://labtether.com/docs/wiki/operations/production-deployment-checklist)
- [Release Readiness Checklist](https://labtether.com/docs/wiki/operations/release-readiness-checklist)
