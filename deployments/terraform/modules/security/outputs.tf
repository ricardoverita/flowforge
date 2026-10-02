output "alb_security_group_id" { value = aws_security_group.alb.id }
output "tasks_security_group_id" { value = aws_security_group.tasks.id }
output "database_security_group_id" { value = aws_security_group.database.id }
output "application_secret_arn" { value = aws_secretsmanager_secret.application.arn }
output "runtime_secret_arn" { value = aws_secretsmanager_secret.runtime.arn }
