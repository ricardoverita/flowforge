module "network" {
  source             = "../../modules/network"
  name               = var.name
  cidr               = var.vpc_cidr
  enable_nat_gateway = var.enable_nat_gateway
}
module "security" {
  source            = "../../modules/security"
  name              = var.name
  vpc_id            = module.network.vpc_id
  allowed_api_cidrs = var.allowed_api_cidrs
}
module "observability" {
  source        = "../../modules/observability"
  name          = var.name
  service_names = var.enable_services ? [for s in ["api", "engine", "worker"] : "${var.name}-${s}"] : []
}
module "database" {
  source             = "../../modules/database"
  name               = var.name
  private_subnet_ids = module.network.private_subnet_ids
  security_group_id  = module.security.database_security_group_id
}
module "compute" {
  source                          = "../../modules/compute"
  name                            = var.name
  region                          = var.region
  vpc_id                          = module.network.vpc_id
  private_subnet_ids              = module.network.private_subnet_ids
  public_subnet_ids               = module.network.public_subnet_ids
  tasks_security_group_id         = module.security.tasks_security_group_id
  alb_security_group_id           = module.security.alb_security_group_id
  log_group_name                  = module.observability.log_group_name
  enable_services                 = var.enable_services
  schema_initialized              = var.schema_initialized
  images                          = var.images
  certificate_arn                 = var.certificate_arn
  nats_url                        = var.nats_url
  database_host                   = module.database.address
  database_credentials_secret_arn = var.database_credentials_secret_arn
  runtime_secret_arn              = var.runtime_secret_arn
  allowed_api_cidrs               = var.allowed_api_cidrs
}
