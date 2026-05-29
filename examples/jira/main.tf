terraform {
  required_providers {
    atlassian = {
      source  = "asymmetric-effort/atlassian"
      version = ">= 0.0.1"
    }
  }
}

provider "atlassian" {
  url      = "https://terraform-example.atlassian.net"
  username = var.atlassian_username
  api_token = var.atlassian_api_token
}

resource "atlassian_jira_space" "project_a" {
  key        = "PRJA"
  name       = "project_a"
  space_type = "classic"
}
