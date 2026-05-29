# Terraform Provider for Atlassian Cloud

<!-- Badges placeholder -->
[![Build Status](https://github.com/asymmetric-effort/terraform-provider-atlassian/actions/workflows/ci.yml/badge.svg)](https://github.com/asymmetric-effort/terraform-provider-atlassian/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.26-blue)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

## Overview

An OpenTofu/Terraform provider for declarative management of Atlassian Cloud
resources across Jira, Confluence, Bitbucket, and Statuspage. This provider
enables infrastructure-as-code workflows for your entire Atlassian Cloud
organization, including identity and access management.

The provider is **OpenTofu-first** (>= 1.6), with Terraform (>= 1.5) as a
secondary target. It ships zero third-party Atlassian SDK dependencies -- all
API communication is handled by a purpose-built thin client with built-in
pagination, rate-limit resilience, and exponential backoff.

## Requirements

- [OpenTofu](https://opentofu.org/) >= 1.6 (primary) or
  [Terraform](https://www.terraform.io/) >= 1.5 (secondary)
- [Go](https://go.dev/) >= 1.26 (for building from source)
- [Docker](https://www.docker.com/) (for running the mock API server during
  development)
- An Atlassian Cloud organization with admin access
- A service account with appropriate API permissions

## Installation

### OpenTofu / Terraform Registry

```hcl
terraform {
  required_providers {
    atlassian = {
      source  = "asymmetric-effort/atlassian"
      version = ">= 0.0.1"
    }
  }
}
```

### Building from Source

```sh
git clone https://github.com/asymmetric-effort/terraform-provider-atlassian.git
cd terraform-provider-atlassian
make build
```

The provider binary is written to `build/terraform-provider-atlassian`.

## Provider Configuration

### Authentication

Two authentication methods are supported. HTTP Basic Auth is **not** supported.

**API Token** -- scoped to a service account:

```hcl
provider "atlassian" {
  url       = "https://your-site.atlassian.net"
  username  = "service-account@example.com"
  api_token = var.atlassian_api_token
}
```

**OAuth 2.0 -- Refresh Token** (user obtains token out-of-band; provider
refreshes automatically):

```hcl
provider "atlassian" {
  url                 = "https://your-site.atlassian.net"
  oauth_client_id     = var.oauth_client_id
  oauth_client_secret = var.oauth_client_secret
  oauth_refresh_token = var.oauth_refresh_token
}
```

**OAuth 2.0 -- Client Credentials** (app-level auth without user context):

```hcl
provider "atlassian" {
  url                 = "https://your-site.atlassian.net"
  oauth_client_id     = var.oauth_client_id
  oauth_client_secret = var.oauth_client_secret
}
```

### Rate-Limit and Retry Configuration

The provider handles HTTP 429 and 503 responses with exponential backoff
and jitter. The following optional attributes control retry behavior:

| Attribute         | Description                              | Default |
|-------------------|------------------------------------------|---------|
| `request_timeout` | Max time for a single API request        | `30s`   |
| `max_retries`     | Max retry attempts on 429/503            | `5`     |
| `retry_wait_min`  | Minimum wait between retries             | `1s`    |
| `retry_wait_max`  | Maximum wait between retries             | `30s`   |

### Environment Variables

All provider attributes can be set via environment variables:

| Environment Variable             | Provider Attribute    |
|----------------------------------|-----------------------|
| `ATLASSIAN_URL`                  | `url`                 |
| `ATLASSIAN_USERNAME`             | `username`            |
| `ATLASSIAN_API_TOKEN`            | `api_token`           |
| `ATLASSIAN_OAUTH_CLIENT_ID`     | `oauth_client_id`     |
| `ATLASSIAN_OAUTH_CLIENT_SECRET`  | `oauth_client_secret` |
| `ATLASSIAN_OAUTH_REFRESH_TOKEN`  | `oauth_refresh_token` |

## Usage Example

```hcl
terraform {
  required_providers {
    atlassian = {
      source  = "asymmetric-effort/atlassian"
      version = ">= 0.0.1"
    }
  }
}

provider "atlassian" {
  url       = var.atlassian_url
  username  = var.atlassian_username
  api_token = var.atlassian_api_token
}

# Create a Jira space (classic project)
resource "atlassian_jira_space" "engineering" {
  key        = "ENG"
  name       = "Engineering"
  space_type = "classic"
}

# Define an issue type
resource "atlassian_jira_issue_type" "bug" {
  name        = "Bug"
  description = "A software defect"
}

# Create a workflow
resource "atlassian_jira_workflow" "simple" {
  name        = "Simple Workflow"
  description = "A simple linear workflow"
}

# Set up a screen
resource "atlassian_jira_screen" "default" {
  name        = "Default Screen"
  description = "Default Jira screen"
}

# Configure a webhook
resource "atlassian_jira_webhook" "ci_notify" {
  name    = "CI Notification"
  url     = "https://ci.example.com/jira-webhook"
  events  = ["jira:issue_created", "jira:issue_updated"]
  enabled = true
}

# Create a Scrum board
resource "atlassian_jira_board" "scrum" {
  name = "Sprint Board"
  type = "scrum"
}

# Read back the space as a data source
data "atlassian_jira_space" "engineering" {
  id = atlassian_jira_space.engineering.id
}
```

## Resources and Data Sources

Every managed resource implements full CRUD operations plus `ImportState` for
`tofu import` / `terraform import`. Each resource type has a corresponding
read-only data source.

### Identity and Access

| Resource                               | Data Source                             |
|----------------------------------------|-----------------------------------------|
| `atlassian_identity_user`              | `atlassian_identity_user`               |
| `atlassian_identity_group`             | `atlassian_identity_group`              |
| `atlassian_identity_group_membership`  | --                                      |
| `atlassian_identity_role`              | `atlassian_identity_role`               |
| `atlassian_identity_role_assignment`   | --                                      |
| `atlassian_identity_token`             | --                                      |

### Jira

| Resource                                          | Data Source                                       |
|---------------------------------------------------|---------------------------------------------------|
| `atlassian_jira_space`                            | `atlassian_jira_space`                            |
| `atlassian_jira_space_component`                  | `atlassian_jira_space_component`                  |
| `atlassian_jira_space_version`                    | `atlassian_jira_space_version`                    |
| `atlassian_jira_custom_domain`                    | `atlassian_jira_custom_domain`                    |
| `atlassian_jira_custom_domain_email`              | `atlassian_jira_custom_domain_email`              |
| `atlassian_jira_issue_type`                       | `atlassian_jira_issue_type`                       |
| `atlassian_jira_issue_type_scheme`                | `atlassian_jira_issue_type_scheme`                |
| `atlassian_jira_issue_link_type`                  | `atlassian_jira_issue_link_type`                  |
| `atlassian_jira_workflow`                         | `atlassian_jira_workflow`                         |
| `atlassian_jira_workflow_scheme`                  | `atlassian_jira_workflow_scheme`                  |
| `atlassian_jira_screen`                           | `atlassian_jira_screen`                           |
| `atlassian_jira_screen_scheme`                    | `atlassian_jira_screen_scheme`                    |
| `atlassian_jira_issue_type_screen_scheme`         | `atlassian_jira_issue_type_screen_scheme`         |
| `atlassian_jira_screen_tab`                       | `atlassian_jira_screen_tab`                       |
| `atlassian_jira_screen_tab_field`                 | --                                                |
| `atlassian_jira_permission_scheme`                | `atlassian_jira_permission_scheme`                |
| `atlassian_jira_security_scheme`                  | `atlassian_jira_security_scheme`                  |
| `atlassian_jira_notification_scheme`              | `atlassian_jira_notification_scheme`              |
| `atlassian_jira_dashboard`                        | `atlassian_jira_dashboard`                        |
| `atlassian_jira_filter`                           | `atlassian_jira_filter`                           |
| `atlassian_jira_custom_field`                     | `atlassian_jira_custom_field`                     |
| `atlassian_jira_field_configuration`              | `atlassian_jira_field_configuration`              |
| `atlassian_jira_field_configuration_scheme`       | `atlassian_jira_field_configuration_scheme`       |
| `atlassian_jira_webhook`                          | `atlassian_jira_webhook`                          |
| `atlassian_jira_board`                            | `atlassian_jira_board`                            |
| `atlassian_jira_priority`                         | `atlassian_jira_priority`                         |
| `atlassian_jira_priority_scheme`                  | `atlassian_jira_priority_scheme`                  |
| `atlassian_jira_automation`                       | `atlassian_jira_automation`                       |
| `atlassian_jira_mail_handler_incoming`            | `atlassian_jira_mail_handler_incoming`            |
| `atlassian_jira_mail_handler_outgoing`            | `atlassian_jira_mail_handler_outgoing`            |

### Confluence

| Resource                                  | Data Source                               |
|-------------------------------------------|-------------------------------------------|
| `atlassian_confluence_space`              | `atlassian_confluence_space`              |
| `atlassian_confluence_space_permission`   | `atlassian_confluence_space_permission`   |
| `atlassian_confluence_page`              | `atlassian_confluence_page`              |
| `atlassian_confluence_page_restriction`   | `atlassian_confluence_page_restriction`   |
| `atlassian_confluence_template`           | `atlassian_confluence_template`           |

### Bitbucket

| Resource                                    | Data Source                                 |
|---------------------------------------------|---------------------------------------------|
| `atlassian_bitbucket_repository`            | `atlassian_bitbucket_repository`            |
| `atlassian_bitbucket_repository_permission` | `atlassian_bitbucket_repository_permission` |
| `atlassian_bitbucket_branch_restriction`    | `atlassian_bitbucket_branch_restriction`    |
| `atlassian_bitbucket_pipeline`              | `atlassian_bitbucket_pipeline`              |
| `atlassian_bitbucket_deployment`            | `atlassian_bitbucket_deployment`            |

### Statuspage

| Resource                                      | Data Source                                   |
|-----------------------------------------------|-----------------------------------------------|
| `atlassian_statuspage_page`                   | `atlassian_statuspage_page`                   |
| `atlassian_statuspage_page_permission`        | `atlassian_statuspage_page_permission`        |
| `atlassian_statuspage_page_incident_template` | `atlassian_statuspage_page_incident_template` |
| `atlassian_statuspage_page_maintenance_template` | `atlassian_statuspage_page_maintenance_template` |
| `atlassian_statuspage_component`              | `atlassian_statuspage_component`              |
| `atlassian_statuspage_component_group`        | `atlassian_statuspage_component_group`        |
| `atlassian_statuspage_subscriber`             | `atlassian_statuspage_subscriber`             |

## Development

### Building

```sh
make build
```

### Linting

Runs `gofmt`, `go vet`, `govulncheck`, `jsonlint`, `yamllint`, and
`markdownlint`:

```sh
make lint
```

### Testing

Tests follow a strict pyramid: unit, integration, end-to-end, and
post-deployment verification (PDV).

```sh
# Run the full test suite
make test

# Run coverage analysis (enforces >= 98% threshold)
make cover
```

PDV tests require live Atlassian credentials and are enabled by setting
`PDV=1` along with the `ATLASSIAN_URL`, `ATLASSIAN_USERNAME`, and
`ATLASSIAN_API_TOKEN` environment variables.

### Mock API

A Dockerized mock Atlassian API server is included for local development and
integration testing:

```sh
make api/build    # Build the mock API Docker image
make api/start    # Start the mock API on http://localhost:8080
make api/stop     # Stop and remove the mock API container
```

### Release

```sh
make release              # Bump patch version, tag locally
make release/patch        # Same as above
make release/minor        # Bump minor version, tag locally
make release/major        # Bump major version, tag locally
```

Tags follow semver with a `v` prefix (e.g., `v0.0.1`) and match the pattern
`^v[0-9]+\.[0-9]+\.[0-9]+$`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on submitting issues and
pull requests.

## License

MIT License -- see [LICENSE](LICENSE) for details.

Copyright (c) 2017-2026 Asymmetric Effort, LLC
