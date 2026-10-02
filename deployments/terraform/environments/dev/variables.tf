variable "region" {
  type    = string
  default = "us-east-1"
}
variable "name" {
  type    = string
  default = "flowforge-dev"
}
variable "vpc_cidr" {
  type    = string
  default = "10.40.0.0/16"
}
variable "enable_nat_gateway" {
  description = "Private tasks need outbound connectivity to an external NATS endpoint. This creates a billable NAT gateway."
  type        = bool
  default     = false
}
variable "enable_services" {
  description = "Create the ALB and ECS services only after images, secrets, NATS and the database schema are ready."
  type        = bool
  default     = false
}
variable "schema_initialized" {
  description = "Confirm a one-off migrate task has applied the schema before services start."
  type        = bool
  default     = false
}
variable "images" {
  description = "Immutable image references for api, engine, worker and migrate."
  type        = map(string)
  default     = {}
  validation {
    condition     = length(var.images) == 0 || alltrue([for s in ["api", "engine", "worker", "migrate"] : can(regex("@sha256:[a-f0-9]{64}$", lookup(var.images, s, "")))])
    error_message = "Supply all four service image references pinned by sha256 digest."
  }
}
variable "nats_url" {
  description = "TLS endpoint of a separately operated persistent NATS JetStream cluster, reachable from private subnets."
  type        = string
  default     = ""
  validation {
    condition     = var.nats_url == "" || can(regex("^tls://", var.nats_url))
    error_message = "Cloud NATS connections must use a tls:// URL."
  }
}
variable "certificate_arn" {
  description = "ACM certificate in this region for the HTTPS ALB listener."
  type        = string
  default     = ""
}
variable "allowed_api_cidrs" {
  description = "Explicit client CIDRs allowed to reach the HTTPS ALB."
  type        = list(string)
  default     = []
}
variable "database_credentials_secret_arn" {
  description = "Existing Secrets Manager JSON secret with a restricted application username and password. The managed RDS master secret is for schema/user administration only."
  type        = string
  default     = ""
}
variable "runtime_secret_arn" {
  description = "Existing Secrets Manager JSON secret with nats_token and api_token. Values are populated outside Terraform."
  type        = string
  default     = ""
}
