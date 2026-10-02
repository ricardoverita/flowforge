resource "aws_cloudwatch_log_group" "services" {
  name              = "/ecs/${var.name}"
  retention_in_days = 14
}
resource "aws_cloudwatch_metric_alarm" "cpu" {
  for_each            = toset(var.service_names)
  alarm_name          = "${each.key}-cpu"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 3
  metric_name         = "CPUUtilization"
  namespace           = "AWS/ECS"
  period              = 60
  statistic           = "Average"
  threshold           = 80
  treat_missing_data  = "notBreaching"
  dimensions          = { ClusterName = var.name, ServiceName = each.key }
  alarm_description   = "Sustained service CPU utilization. Attach an SNS action for your environment."
}
