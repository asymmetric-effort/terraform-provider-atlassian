output "project_a_id" {
  description = "The ID of project_a"
  value       = atlassian_jira_space.project_a.id
}

output "project_a_key" {
  description = "The key of project_a"
  value       = atlassian_jira_space.project_a.key
}

output "project_a_url" {
  description = "The browse URL of project_a"
  value       = atlassian_jira_space.project_a.browse_url
}

output "issue_type_id" {
  description = "The ID of the Bug issue type"
  value       = atlassian_jira_issue_type.bug.id
}

output "issue_link_type_id" {
  description = "The ID of the Blocks link type"
  value       = atlassian_jira_issue_link_type.blocks.id
}

output "webhook_id" {
  description = "The ID of the CI notification webhook"
  value       = atlassian_jira_webhook.ci_notify.id
}
