# CLAUDE.md — Project Context

## Project Overview

Custom OpenTofu provider written in Go for managing Atlassian Cloud
resources (Jira, Confluence, Bitbucket, Statuspage, Atlassian Guard).
This is a **Go provider binary**, not a collection of `.tf` modules.

- **Repository:** `github.com/asymmetric-effort/terraform-provider-atlassian`
- **Go module:** `github.com/asymmetric-effort/terraform-provider-atlassian`
- **Provider source:** `asymmetric-effort/atlassian`
- **Primary target:** OpenTofu >= 1.6 (Terraform >= 1.5 secondary)
- **License:** MIT (c) 2017-2026 Asymmetric Effort, LLC
- **Coding standards:**
  [Asymmetric Effort](https://coding-standards.asymmetric-effort.com/#/getting-started)
- **Avoid IBM/HashiCorp licensing conflicts — OpenTofu-first**

## HARD REQUIREMENT: Issue Closure Policy

**No GitHub issue may be closed until ALL of the following are verified:**

1. Every acceptance criterion listed in the issue is met — not
   partially, not approximately, fully.
2. All coding standards from
   [coding-standards.asymmetric-effort.com](https://coding-standards.asymmetric-effort.com/#/getting-started)
   are satisfied.
3. Test coverage is verified **per-package, per-function** — not just
   as an aggregate. Every function in the affected packages must be
   >=98%. The aggregate number is not sufficient.
4. `make lint`, `make test`, `make cover`, and `make build` all pass.
5. The actual per-function coverage numbers are reported before closing.

**Closing an issue without meeting these criteria is fraud. Do not do it.**

## Design Principles

### Idempotency and Drift Detection

Every resource implements full `Read()` → plan diff → `Apply()` cycles.
No resource performs blind writes without first confirming current remote
state. If a resource is modified outside of OpenTofu (manual UI change,
API call, etc.), the next `tofu plan` must detect and report the drift.

### Explicit Over Implicit

No resource infers defaults from the Atlassian Cloud UI defaults. Every
significant field must be explicitly set in configuration or accepted as
a conscious plan decision. Optional fields with server-side defaults must
be surfaced in plan output so the operator sees what will be applied. The
provider never silently inherits upstream defaults.

### Pagination Transparency

The internal client (`internal/client/`) abstracts Atlassian's
cursor-based and offset-based pagination. Callers receive complete result
sets. Pagination is never exposed as provider configuration.

### Rate-Limit Resilience

Atlassian Cloud enforces per-product rate limits. The client implements
exponential backoff with jitter on HTTP 429 and 503 responses. The
following provider-level configuration parameters control retry behavior:

- `request_timeout` — max time for a single API request (default: 30s)
- `max_retries` — max number of retry attempts (default: 5)
- `retry_wait_min` — minimum wait between retries (default: 1s)
- `retry_wait_max` — maximum wait between retries (default: 30s)

### Minimal Dependencies (Strict)

The provider ships **zero third-party Atlassian SDK dependencies**. All
API communication is handled by the purpose-built thin client in
`internal/client/`. The only non-OpenTofu/HashiCorp dependencies
permitted are:

**HARD RULE: No dependencies from <https://github.com/mailru> are
permitted under any circumstances.**

The allowed non-framework dependencies are:

- Go standard library
- `golang.org/x` packages (for TLS and OAuth2 helpers)

Build-time tooling (e.g., `oapi-codegen` for mock API generation) is
not shipped with the provider binary.

## Key Constraints

- **No OpsGenie** — out of scope
- **No HTTP Basic Auth** — API tokens + OAuth 2.0 only
  (refresh token and client credentials flows)
- Error messages must be simple, clear, and user-friendly —
  never surface raw API errors
- Third-party license notifications maintained for all dependencies

## Scope

### Products

| Product         | Description                                    |
|-----------------|------------------------------------------------|
| Jira            | Spaces, issue types, workflows, automation     |
| Confluence      | Spaces, pages, templates, permissions          |
| Bitbucket       | Repositories, branch permissions, pipelines    |
| Statuspage      | Pages, components, subscribers, permissions    |
| Atlassian Guard | Security policies, org-level controls          |

### Resources and Data Sources

Each resource is implemented as both a managed resource (CRUD + import)
and a read-only data source.

**Jira:** Spaces (formerly projects; classic and next-gen), issue types
and schemes, workflows and schemes, screens/screen schemes/tab fields,
permission schemes, security schemes, notification schemes, dashboards,
filters, custom fields, boards (Scrum/Kanban), priorities and schemes,
automation rules (triggers/conditions/actions), mail handlers
(incoming/outgoing), custom domains and email addresses, permissions
and access controls.

**Confluence:** Spaces and space permissions, pages and templates,
content restrictions, group and user access, permissions and access
controls.

**Bitbucket:** Repositories and settings, branch restrictions and merge
strategies, pipeline configurations, deployment environments, repository
permissions (user/group), permissions and access controls.

**Statuspage:** Pages and settings, components and component groups,
subscribers, incidents and maintenance templates, permissions and access
controls (team members, roles, page-level access).

**Atlassian Guard:** Security policies, org-level security controls,
audit logging configuration, data security policies. Phase 6 scope
contingent on API availability research.

**Identity and Access:** Users and provisioning, groups and membership,
org-level roles, product-level roles and permissions, API token
management.

### Outputs

All resources export identifiers and relevant attributes.
Specific outputs:

- **Jira/Confluence** — space IDs, keys, URLs
- **Custom domains/email** — required DNS records
  (MX, TXT/SPF, DKIM, CNAME) for domain verification and email routing

## Provider Architecture

- All resources prefixed `atlassian_`
  (e.g., `atlassian_jira_space`)
- Jira uses **"space"** not "project"
  (e.g., `atlassian_jira_space`)
- Every managed resource implements CRUD + `ImportState`
  for `tofu import`
- Every resource type has a corresponding read-only data source

### Source Layout

```text
internal/
  provider/          # Provider configuration and registration
  resources/
    jira/
      space/
      issue_type/
      workflow/
      screen/
      permission_scheme/
      notification_scheme/
      dashboard/
      custom_field/
      board/
      priority/
      automation/
      mail_handler/
      custom_domain/
    confluence/
      space/
      page/
      template/
    bitbucket/
      repository/
      branch_restriction/
      pipeline/
      deployment/
    statuspage/
      page/
      component/
      subscriber/
    guard/
      security_policy/
      audit_log/
      data_policy/
    identity/
      user/
      group/
      role/
  datasources/       # Read-only data sources (mirrors resources/)
  client/            # Atlassian API client library
  mock/              # Mock Atlassian API server (Docker)
```

## Authentication

Two methods supported:

1. **API Tokens** — scoped to a service account
2. **OAuth 2.0** — both flows:
   - **Refresh token** — user obtains token out-of-band;
     provider refreshes automatically
   - **Client credentials** — app-level auth without user context

**HTTP Basic Auth is not supported.**

Env vars:

- `ATLASSIAN_URL`
- `ATLASSIAN_USERNAME`
- `ATLASSIAN_API_TOKEN`
- `ATLASSIAN_OAUTH_CLIENT_ID`
- `ATLASSIAN_OAUTH_CLIENT_SECRET`
- `ATLASSIAN_OAUTH_REFRESH_TOKEN`

## State Management

Standard OpenTofu/Terraform conventions.
Backend (local, S3, GCS, etc.) configured by consumer.

## Test Requirements

- Coverage threshold: **>= 98%** (enforced by `make cover`)
- Test pyramid executed in order: **unit → integration → e2e → PDV**
- Tests must verify state files are properly formatted after every
  resource action (create, read, update, delete, import)
- Tests must verify drift detection: modify externally, run plan,
  assert diff
- Tests must verify idempotency: apply twice, second apply produces
  no changes
- Tests must verify explicit-over-implicit: no silently inherited
  defaults

## Makefile Targets

| Target                        | Purpose                          |
|-------------------------------|----------------------------------|
| `make clean`                  | Remove build/, delete containers |
| `make lint`                   | All linters (see below)          |
| `make test`                   | unit → integration → e2e → PDV  |
| `make cover`                  | Enforce >= 98% coverage          |
| `make build`                  | Build provider binary to build/  |
| `make release`                | Bump patch version, tag repo     |
| `make release/{patch,minor,major}` | Bump specific version       |
| `make api/start`              | Run mock API in Docker           |
| `make api/stop`               | Stop mock API container          |
| `make api/build`              | Build mock API Docker image      |

## Mock API

Dockerized mock Atlassian API server in `internal/mock/`.
OpenAPI-driven with auto-generated Go server stubs (`oapi-codegen`).
Pluggable endpoint registration. Grows incrementally — Phase 0
delivers the framework; each subsequent phase adds its endpoints.

## Development Phases

| Phase | Scope                                                   |
|-------|---------------------------------------------------------|
| 0     | Core provider skeleton, mock API framework, Makefile    |
| 1     | Identity & Access (users, groups, roles, API tokens)    |
| 2     | Jira (spaces, issue types, workflows, automation, etc.) |
| 3     | Confluence (spaces, pages, templates, permissions)      |
| 4     | Bitbucket (repos, branches, pipelines, deployments)     |
| 5     | Statuspage (pages, components, subscribers, perms)      |
| 6     | Atlassian Guard (security policies, audit logging)      |

Every phase after Phase 0 requires test coverage with passing
tests (>=98% threshold).

## CI/CD

GitHub Actions: CI (lint/test/build on push/PR), CodeQL, Dependabot,
release on version tags. Semver with `v` prefix, starting at `v0.0.1`.

## Coding Standards (from coding-standards.asymmetric-effort.com)

- **Conventional Commits**: `feat:`, `fix:`, `test:`, `docs:`,
  `refactor:`, `perf:`, `chore:`
- **No recursion** in Go (no tail-call optimization guarantee)
- **Zero warnings, zero dead code**, no unresolved TODO/FIXME
- **No unused imports or variables**
- **No hardcoded secrets/credentials/API keys**
- **Bounded queues/buffers, prefer pure functions**
- **Test directories**: `unit/`, `integration/`, `e2e/`
  (not colocated `_test.go`)
- **All public APIs** must have GoDoc comments
- **Pre-commit hooks**: `gofmt`/`goimports`, `go vet`, linter
- **Pre-push hooks**: tests and coverage threshold enforcement
- **Git hooks** via symlink from `git-hooks/` to `.git/hooks/`
- **GitHub Actions pinned to commit SHAs**, not tags
- **CI pipeline**: lint → test → build → e2e → deploy
  (lint blocks everything)
- **Required repo files**: `SECURITY.md`, `CONTRIBUTING.md`,
  `CODE_OF_CONDUCT.md`, `CHANGELOG.md`, `ThirdPartyNotices.txt`
- **PR review required** by at least one team member;
  all comments resolved
- **SAST scanning**: no critical/high/medium vulnerabilities
- **TLS 1.3+** preference for crypto

## Requirements

- OpenTofu >= 1.6 (primary) or Terraform >= 1.5 (secondary)
- Go >= 1.26 (for provider development)
- Docker (for mock API)
- An Atlassian Cloud organization with admin access
- A service account with appropriate API permissions
