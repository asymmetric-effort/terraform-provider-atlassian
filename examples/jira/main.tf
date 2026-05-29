terraform {
  required_providers {
    atlassian = {
      source  = "asymmetric-effort/atlassian"
      version = ">= 0.0.2"
    }
  }
}

provider "atlassian" {
  url       = "https://terraform-example.atlassian.net"
  username  = var.atlassian_username
  api_token = var.atlassian_api_token
}

# --- Jira Space ---

resource "atlassian_jira_space" "project_a" {
  key        = "PRJA"
  name       = "project_a"
  space_type = "classic"
}

# --- Issue Type ---

resource "atlassian_jira_issue_type" "bug" {
  name        = "Bug"
  description = "A software defect"
}

# --- Workflow ---

resource "atlassian_jira_workflow" "simple" {
  name        = "Simple Workflow"
  description = "A simple linear workflow"
}

# --- Screen ---

resource "atlassian_jira_screen" "default" {
  name        = "Default Screen"
  description = "Default Jira screen"
}

# --- Field Configuration ---

resource "atlassian_jira_field_configuration" "standard" {
  name        = "Standard Field Config"
  description = "Standard field configuration"
}

resource "atlassian_jira_field_configuration_scheme" "standard" {
  name        = "Standard FC Scheme"
  description = "Standard field configuration scheme"
}

# --- Issue Link Type ---

resource "atlassian_jira_issue_link_type" "blocks" {
  name    = "Blocks"
  inward  = "is blocked by"
  outward = "blocks"
}

# --- Webhook ---

resource "atlassian_jira_webhook" "ci_notify" {
  name    = "CI Notification"
  url     = "https://ci.example.com/jira-webhook"
  events  = ["jira:issue_created", "jira:issue_updated"]
  enabled = true
}

# --- Board ---

resource "atlassian_jira_board" "scrum" {
  name = "Sprint Board"
  type = "scrum"
}

# --- Data Sources ---

data "atlassian_jira_space" "project_a" {
  id = atlassian_jira_space.project_a.id
}

data "atlassian_jira_issue_link_type" "blocks" {
  id = atlassian_jira_issue_link_type.blocks.id
}
