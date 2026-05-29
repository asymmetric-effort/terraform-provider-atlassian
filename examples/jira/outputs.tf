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
