output "cluster_name" { value = aws_ecs_cluster.this.name }
output "repositories" { value = { for k, r in aws_ecr_repository.service : k => r.repository_url } }
output "alb_dns_name" { value = try(aws_lb.api[0].dns_name, null) }
output "migrate_task_definition_arn" { value = try(aws_ecs_task_definition.service["migrate"].arn, null) }
