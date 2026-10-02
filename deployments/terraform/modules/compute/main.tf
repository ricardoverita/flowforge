locals {
  services = { api = 8080, engine = 8081, worker = 8082 }
}
resource "aws_ecs_cluster" "this" {
  name = var.name
  setting {
    name  = "containerInsights"
    value = "enabled"
  }
}
resource "aws_ecr_repository" "service" {
  for_each             = toset(["api", "engine", "worker", "migrate"])
  name                 = "${var.name}-${each.key}"
  image_tag_mutability = "IMMUTABLE"
  image_scanning_configuration {
    scan_on_push = true
  }
  encryption_configuration {
    encryption_type = "AES256"
  }
}
resource "aws_ecr_lifecycle_policy" "service" {
  for_each   = aws_ecr_repository.service
  repository = each.value.name
  policy = jsonencode({ rules = [{
    rulePriority = 1
    description  = "Expire untagged images after seven days"
    selection    = { tagStatus = "untagged", countType = "sinceImagePushed", countUnit = "days", countNumber = 7 }
    action       = { type = "expire" }
  }] })
}
data "aws_iam_policy_document" "assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}
resource "aws_iam_role" "execution" {
  name               = "${var.name}-execution"
  assume_role_policy = data.aws_iam_policy_document.assume.json
}
data "aws_iam_policy_document" "execution" {
  statement {
    actions   = ["ecr:GetAuthorizationToken"]
    resources = ["*"]
  }
  statement {
    actions   = ["ecr:BatchCheckLayerAvailability", "ecr:GetDownloadUrlForLayer", "ecr:BatchGetImage"]
    resources = [for r in aws_ecr_repository.service : r.arn]
  }
  statement {
    actions   = ["logs:CreateLogStream", "logs:PutLogEvents"]
    resources = ["arn:aws:logs:${var.region}:${data.aws_caller_identity.current.account_id}:log-group:${var.log_group_name}:*"]
  }
  dynamic "statement" {
    for_each = var.database_credentials_secret_arn != "" && var.runtime_secret_arn != "" ? [1] : []
    content {
      actions   = ["secretsmanager:GetSecretValue"]
      resources = [var.database_credentials_secret_arn, var.runtime_secret_arn]
    }
  }
}
data "aws_caller_identity" "current" {}
resource "aws_iam_role_policy" "execution" {
  name   = "runtime"
  role   = aws_iam_role.execution.id
  policy = data.aws_iam_policy_document.execution.json
}
resource "aws_iam_role" "task" {
  name               = "${var.name}-task"
  assume_role_policy = data.aws_iam_policy_document.assume.json
}
resource "aws_ecs_task_definition" "service" {
  for_each                 = var.images
  family                   = "${var.name}-${each.key}"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task.arn
  runtime_platform {
    cpu_architecture        = "X86_64"
    operating_system_family = "LINUX"
  }
  container_definitions = jsonencode([merge({
    name                   = each.key
    image                  = each.value
    essential              = true
    user                   = "65532:65532"
    readonlyRootFilesystem = true
    linuxParameters        = { capabilities = { drop = ["ALL"] } }
    portMappings           = each.key == "migrate" ? [] : [{ containerPort = local.services[each.key], protocol = "tcp" }]
    environment = [
      { name = "SERVICE_NAME", value = "flowforge-${each.key}" },
      { name = "HTTP_ADDR", value = ":${lookup(local.services, each.key, 8080)}" },
      { name = "DATABASE_HOST", value = var.database_host },
      { name = "DATABASE_PORT", value = "5432" },
      { name = "DATABASE_NAME", value = "flowforge" },
      { name = "DATABASE_SSLMODE", value = "require" },
      { name = "NATS_URL", value = var.nats_url }
    ]
    secrets = [
      { name = "DATABASE_USER", valueFrom = "${var.database_credentials_secret_arn}:username::" },
      { name = "DATABASE_PASSWORD", valueFrom = "${var.database_credentials_secret_arn}:password::" },
      { name = "NATS_TOKEN", valueFrom = "${var.runtime_secret_arn}:nats_token::" },
      { name = "API_TOKEN", valueFrom = "${var.runtime_secret_arn}:api_token::" }
    ]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = var.log_group_name
        awslogs-region        = var.region
        awslogs-stream-prefix = each.key
      }
    }
    }, each.key == "migrate" ? {} : {
    healthCheck = { command = ["CMD", "/flowforge", "healthcheck"], interval = 15, timeout = 5, retries = 3, startPeriod = 30 }
  })])
  lifecycle {
    precondition {
      condition     = var.database_credentials_secret_arn != "" && var.runtime_secret_arn != "" && var.nats_url != ""
      error_message = "Task definitions require populated application/runtime secrets and a reachable TLS NATS endpoint."
    }
  }
}
resource "aws_lb" "api" {
  count                      = var.enable_services ? 1 : 0
  name                       = var.name
  load_balancer_type         = "application"
  internal                   = false
  security_groups            = [var.alb_security_group_id]
  subnets                    = var.public_subnet_ids
  drop_invalid_header_fields = true
  lifecycle {
    precondition {
      condition     = var.certificate_arn != "" && length(var.allowed_api_cidrs) > 0 && var.schema_initialized && length(var.images) == 4
      error_message = "Services require an ACM certificate, explicit client CIDRs, four image digests and an initialized database schema."
    }
  }
}
resource "aws_lb_target_group" "api" {
  count       = var.enable_services ? 1 : 0
  name        = "${var.name}-api"
  port        = 8080
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = var.vpc_id
  health_check {
    path                = "/ready"
    matcher             = "200"
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }
}
resource "aws_lb_listener" "https" {
  count             = var.enable_services ? 1 : 0
  load_balancer_arn = aws_lb.api[0].arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = var.certificate_arn
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.api[0].arn
  }
}
resource "aws_ecs_service" "service" {
  for_each                           = var.enable_services ? local.services : {}
  name                               = "${var.name}-${each.key}"
  cluster                            = aws_ecs_cluster.this.id
  task_definition                    = aws_ecs_task_definition.service[each.key].arn
  desired_count                      = 1
  launch_type                        = "FARGATE"
  platform_version                   = "1.4.0"
  wait_for_steady_state              = true
  deployment_minimum_healthy_percent = 100
  deployment_maximum_percent         = 200
  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }
  network_configuration {
    subnets          = var.private_subnet_ids
    security_groups  = [var.tasks_security_group_id]
    assign_public_ip = false
  }
  dynamic "load_balancer" {
    for_each = each.key == "api" ? [1] : []
    content {
      target_group_arn = aws_lb_target_group.api[0].arn
      container_name   = "api"
      container_port   = 8080
    }
  }
  depends_on = [aws_lb_listener.https, aws_iam_role_policy.execution]
}
