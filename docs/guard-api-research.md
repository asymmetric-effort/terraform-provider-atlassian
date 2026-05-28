# Atlassian Guard API Research

## Date

2026-05-28

## Summary

This document assesses the Atlassian Guard/Admin API availability
for each planned Phase 6 resource type.

## API Documentation Sources

- Atlassian Organization REST API:
  `developer.atlassian.com/cloud/admin/organization/rest/`
- Postman collection:
  `developer.atlassian.com/cloud/admin/organization/organization.postman.json`

## Authentication Requirements

All Atlassian Admin/Guard APIs require:

- Bearer token (API key) or OAuth 2.0
- Org-level admin privileges
- Our existing auth methods are compatible

## Rate Limits

- Standard Atlassian Cloud rate limits apply (HTTP 429)
- No Guard-specific rate limit documentation found
- Our existing backoff with jitter handles 429/503

## Resource Assessment

### 1. Security Policies (#72)

API availability: PARTIAL — Tag exists but no endpoints defined.

The Organization REST API declares a "Policies" tag but the
OpenAPI spec contains no actual policy endpoint paths.

Recommendation: Cannot implement full CRUD. Defer until
Atlassian publishes the endpoints.

CRUD assessment:

- Create: NOT AVAILABLE
- Read: UNCERTAIN
- Update: NOT AVAILABLE
- Delete: NOT AVAILABLE

### 2. Audit Log Configuration (#73)

API availability: PARTIAL — Events API exists (read-only).

The Organization REST API has an "Events" API group with
likely endpoints at:

- `GET /v1/orgs/{orgId}/events` — list audit events

This is read-only. No config API for retention or export.

Recommendation: Implement a read-only data source for
querying audit events. No managed resource.

CRUD assessment:

- Create: NOT AVAILABLE
- Read: AVAILABLE (events listing)
- Update: NOT AVAILABLE
- Delete: NOT AVAILABLE

### 3. Data Security Policies (#74)

API availability: NOT AVAILABLE.

No data security policy endpoints found anywhere. The Data
Loss Prevention REST API returned 404.

Recommendation: Cannot implement. Document and skip.

CRUD assessment:

- Create: NOT AVAILABLE
- Read: NOT AVAILABLE
- Update: NOT AVAILABLE
- Delete: NOT AVAILABLE

### 4. Org-Level Security Controls (#75)

API availability: PARTIAL.

App Access Settings has full CRUD:

- `GET /v2/orgs/{orgId}/app-access-settings/domains` — list
- `POST /v2/orgs/{orgId}/app-access-settings/domains` — create
- `GET .../domains/{domain}` — read
- `PUT .../domains/{domain}` — update
- `POST .../domains/{domain}/products` — partial update

User management is already covered by Phase 1.

Recommendation: Implement `atlassian_guard_app_access_policy`.

CRUD assessment (App Access):

- Create: AVAILABLE
- Read: AVAILABLE
- Update: AVAILABLE
- Delete: NOT DOCUMENTED

## Confirmed Implementable Resources

| Resource | Type | API Support |
|---|---|---|
| `atlassian_guard_app_access_policy` | Managed | App access settings CRUD |
| `atlassian_guard_audit_event` | Data source | Read-only events |

## Resources That Cannot Be Implemented

| Resource | Reason |
|---|---|
| Security policies (#72) | Endpoints not published |
| Audit log configuration (#73) | No config API |
| Data security policies (#74) | No API surface |

## Scope Recommendation

Phase 6 should be reduced to:

1. `atlassian_guard_app_access_policy` — managed resource
2. `atlassian_guard_audit_event` — read-only data source
3. Mock API, OpenAPI spec, error messages, and tests
4. Remaining resources deferred until APIs are published
