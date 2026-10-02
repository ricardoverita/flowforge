output "cluster_name" {
  value = module.compute.cluster_name
}
output "repositories" {
  value = module.compute.repositories
}
output "database_address" {
  value = module.database.address
}
output "database_master_secret_arn" {
  value = module.database.master_secret_arn
}
output "application_secret_arn" {
  value = module.security.application_secret_arn
}
output "runtime_secret_arn" {
  value = module.security.runtime_secret_arn
}
output "alb_dns_name" {
  value = module.compute.alb_dns_name
}
output "migrate_task_definition_arn" {
  value = module.compute.migrate_task_definition_arn
}
output "task_subnets" {
  value = module.network.private_subnet_ids
}
output "task_security_group_id" {
  value = module.security.tasks_security_group_id
}
