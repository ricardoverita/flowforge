resource "aws_security_group" "alb" {
  name   = "${var.name}-alb"
  vpc_id = var.vpc_id
}
resource "aws_vpc_security_group_ingress_rule" "https" {
  for_each          = toset(var.allowed_api_cidrs)
  security_group_id = aws_security_group.alb.id
  cidr_ipv4         = each.value
  from_port         = 443
  to_port           = 443
  ip_protocol       = "tcp"
}
resource "aws_security_group" "tasks" {
  name   = "${var.name}-tasks"
  vpc_id = var.vpc_id
}
resource "aws_vpc_security_group_ingress_rule" "api" {
  security_group_id            = aws_security_group.tasks.id
  referenced_security_group_id = aws_security_group.alb.id
  from_port                    = 8080
  to_port                      = 8080
  ip_protocol                  = "tcp"
}
resource "aws_vpc_security_group_egress_rule" "alb" {
  security_group_id            = aws_security_group.alb.id
  referenced_security_group_id = aws_security_group.tasks.id
  from_port                    = 8080
  to_port                      = 8080
  ip_protocol                  = "tcp"
}
# Tasks contact RDS, AWS private endpoints and the selected NATS endpoint.
# Tighten this to the actual network ranges before a production deployment.
resource "aws_vpc_security_group_egress_rule" "tasks" {
  security_group_id = aws_security_group.tasks.id
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "-1"
}
resource "aws_security_group" "database" {
  name   = "${var.name}-database"
  vpc_id = var.vpc_id
}
resource "aws_vpc_security_group_ingress_rule" "postgres" {
  security_group_id            = aws_security_group.database.id
  referenced_security_group_id = aws_security_group.tasks.id
  from_port                    = 5432
  to_port                      = 5432
  ip_protocol                  = "tcp"
}
resource "aws_secretsmanager_secret" "application" {
  name                    = "${var.name}/database-application"
  recovery_window_in_days = 7
  description             = "Restricted PostgreSQL application username/password; populated outside Terraform."
}
resource "aws_secretsmanager_secret" "runtime" {
  name                    = "${var.name}/runtime"
  recovery_window_in_days = 7
  description             = "NATS token and API token; populated outside Terraform."
}
