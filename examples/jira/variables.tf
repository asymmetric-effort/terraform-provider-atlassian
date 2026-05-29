variable "atlassian_url" {
  description = "Atlassian Cloud site URL (e.g., https://your-site.atlassian.net)"
  type        = string
}

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
