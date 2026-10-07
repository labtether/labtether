# Security Policy

For LabTether's technical security posture, threat model, transport rules, and secrets handling, see [docs/SECURITY.md](docs/SECURITY.md).

## Supported Versions

| Version line | Status |
|---|---|
| `main` | Supported for active development snapshots |
| Latest tagged release | Security reports accepted; each fix record states its release status |
| Older snapshots and ad-hoc pre-release builds | Best effort only |

## Reporting A New Vulnerability

[Report a vulnerability privately](https://github.com/labtether/labtether/security/advisories/new).

Use this private channel for a suspected vulnerability that has not already been publicly disclosed. Reports may cover the Hub, console, agents, CLI, protocol, Home Assistant integration, website, or iOS app; name the affected component and repository.

Include:

- affected version or commit and deployment path
- impact summary and reproduction steps
- logs or screenshots with secrets redacted

Initial acknowledgement target: within 5 business days. Coordinated disclosure is preferred. Please avoid public disclosure until a fix or mitigation is available, unless a coordinated timeline is agreed.

## Public CVE And Security Fix Records

- [Closed CVE issues](https://github.com/labtether/labtether/issues?q=is%3Aissue%20is%3Aclosed%20label%3Acve).
- [All public security issues](https://github.com/labtether/labtether/issues?q=is%3Aissue%20label%3Asecurity).
- [Published LabTether security advisories](https://github.com/labtether/labtether/security/advisories).
- [File a public CVE or security fix record](https://github.com/labtether/labtether/issues/new?template=security-remediation.yml).

Public issues can track an already disclosed CVE or document a verified security fix. New, undisclosed vulnerabilities belong in the private reporting channel above.

Use `security` for security records and add `cve` only when an assigned CVE ID has been verified against its published advisory. For dependency updates, credit the upstream advisory and identify the dependency; do not present the CVE as a newly assigned LabTether vulnerability.

Each public record must include:

1. The published CVE or advisory, when one exists, and the affected component.
2. The affected dependency or source version and the verified patched version or fix commit.
3. A merged pull request or commit and public source links showing the fix.
4. The checks actually performed, with public CI links when available.
5. The release or deployment status. A source fix on `main` does not establish that a tagged release, container image, or installed deployment contains it.

Close an issue as **completed** only after its specific fix has been verified. Keep unresolved work open; dismissing an alert does not prove remediation. If a record is added after the fix, say that it is retrospective and give the real fix date. Dependency presence alone does not establish that LabTether was exploitable or that a deployment was compromised.

For a LabTether-specific vulnerability, maintainers may publish a GitHub Security Advisory and request a CVE when appropriate. Existing upstream CVEs keep their original identifiers and attribution.

## Non-Security Bugs

For ordinary bugs, setup help, or usage questions, use [SUPPORT.md](SUPPORT.md).
