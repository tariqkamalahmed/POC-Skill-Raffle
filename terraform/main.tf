terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.24.0"
    }
  }
}

provider "aws" {
  region                   = "eu-west-2"
  shared_config_files      = ["~/.aws/config"]
  shared_credentials_files = ["~/.aws/credentials"]
}

module "services" {
  source        = "./modules/services"
  raffle_prefix = var.raffle_prefix
}

# ECR Repository for Jenkins pipeline image builds
resource "aws_ecr_repository" "raffle_repo" {
  name                 = "${var.raffle_prefix}-backend"
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = {
    Name = "${var.raffle_prefix}-repo"
  }
}

resource "aws_vpc" "raffle-test-vpc" {
  cidr_block           = "172.16.0.0/16"
  enable_dns_hostnames = true

  tags = {
    Name = "${var.raffle_prefix}-vpc"
  }
}

resource "aws_internet_gateway" "raffle-test-igw" {
  vpc_id = aws_vpc.raffle-test-vpc.id

  tags = {
    Name = "${var.raffle_prefix}-igw"
  }
}

resource "aws_subnet" "raffle-test-subnet1-public" {
  vpc_id                  = aws_vpc.raffle-test-vpc.id
  cidr_block              = "172.16.10.0/24"
  availability_zone       = "eu-west-2a"
  map_public_ip_on_launch = true
  tags = {
    Name = "${var.raffle_prefix}-subnet1-public"
  }
}

resource "aws_subnet" "raffle-test-subnet2-public" {
  vpc_id                  = aws_vpc.raffle-test-vpc.id
  cidr_block              = "172.16.20.0/24"
  availability_zone       = "eu-west-2b"
  map_public_ip_on_launch = true

  tags = {
    Name = "${var.raffle_prefix}-subnet2-public"
  }
}

resource "aws_route_table" "raffle-test-public1-route" {
  vpc_id = aws_vpc.raffle-test-vpc.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.raffle-test-igw.id
  }
  tags = {
    Name = "${var.raffle_prefix}-gateway-route1"
  }
}

resource "aws_route_table" "raffle-test-public2-route" {
  vpc_id = aws_vpc.raffle-test-vpc.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.raffle-test-igw.id
  }
  tags = {
    Name = "${var.raffle_prefix}-gateway-route2"
  }
}

resource "aws_route_table_association" "public1" {
  subnet_id      = aws_subnet.raffle-test-subnet1-public.id
  route_table_id = aws_route_table.raffle-test-public1-route.id
}

resource "aws_route_table_association" "public2" {
  subnet_id      = aws_subnet.raffle-test-subnet2-public.id
  route_table_id = aws_route_table.raffle-test-public2-route.id
}

resource "aws_db_subnet_group" "raffle-test-subnet-group" {
  name       = "${var.raffle_prefix}-subnet-group"
  subnet_ids = [aws_subnet.raffle-test-subnet1-public.id, aws_subnet.raffle-test-subnet2-public.id]

  tags = {
    Name = "${var.raffle_prefix}-subnet-group"
  }
}

resource "random_password" "raffle-test-db-password" {
  length  = 32
  upper   = true
  numeric = true
  special = false
}

data "http" "my_public_ip" {
  url = "https://ifconfig.me/ip"
}

resource "aws_security_group" "raffle-test-sg" {
  vpc_id      = aws_vpc.raffle-test-vpc.id
  name        = "${var.raffle_prefix}-sg"
  description = "Allow Postgres traffic and standard outbound connectivity"

  ingress {
    from_port   = 5432
    to_port     = 5432
    protocol    = "tcp"
    cidr_blocks = ["${data.http.my_public_ip.response_body}/32", "172.16.0.0/16"]
    description = "Allow Postgres from local admin IP and VPC"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Allow all outbound traffic"
  }
}

resource "aws_db_instance" "raffle-test-pgsql" {
  identifier              = "${var.raffle_prefix}-pgsql"
  storage_encrypted       = true
  instance_class          = "db.t3.micro"
  allocated_storage       = 5
  engine                  = "postgres"
  engine_version          = "15.3"
  db_name                 = "raffle" # Matches DB_NAME in main.go
  skip_final_snapshot     = true
  publicly_accessible     = true
  vpc_security_group_ids  = [aws_security_group.raffle-test-sg.id]
  db_subnet_group_name    = aws_db_subnet_group.raffle-test-subnet-group.name
  username                = "postgres"
  backup_retention_period = 2
  password                = random_password.raffle-test-db-password.result
  tags = {
    Name = "${var.raffle_prefix}-pgsql"
  }
  depends_on = [module.services]
}