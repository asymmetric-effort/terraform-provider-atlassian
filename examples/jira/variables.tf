variable "atlassian_username" {
  description = "Atlassian service account email"
  type        = string
  sensitive   = true
}

variable "atlassian_api_token" {
  description = "Atlassian API token"
  type        = string
  sensitive   = true
}
