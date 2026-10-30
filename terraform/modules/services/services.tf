data "aws_caller_identity" "current" {}
data "aws_region" "current" {}

resource "aws_vpc" "raffle-services-vpc" {
  cidr_block           = "172.16.0.0/16"
  enable_dns_hostnames = true

  tags = {
    Name = "${var.raffle_prefix}-services-vpc"
  }
}

resource "aws_internet_gateway" "raffle-services-igw" {
  vpc_id = aws_vpc.raffle-services-vpc.id

  tags = {
    Name = "${var.raffle_prefix}-services-igw"
  }
}

resource "aws_subnet" "raffle-services-subnet-public" {
  vpc_id                  = aws_vpc.raffle-services-vpc.id
  cidr_block              = "172.16.15.0/24"
  availability_zone       = "eu-west-2a"
  map_public_ip_on_launch = true
  tags = {
    Name = "${var.raffle_prefix}-services-subnet-public"
  }
}

resource "aws_route_table" "raffle-services-public-route" {
  vpc_id = aws_vpc.raffle-services-vpc.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.raffle-services-igw.id
  }
  tags = {
    Name = "${var.raffle_prefix}-services-gateway-route"
  }
}

resource "aws_route_table_association" "services-public" {
  subnet_id      = aws_subnet.raffle-services-subnet-public.id
  route_table_id = aws_route_table.raffle-services-public-route.id
}

data "http" "services_public_ip" {
  url = "https://ifconfig.me/ip"
}

resource "aws_security_group" "raffle-services-sg" {
  vpc_id      = aws_vpc.raffle-services-vpc.id
  name        = "${var.raffle_prefix}-services-sg"
  description = "Allow inbound web and management traffic for raffle backend services"

  ingress {
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Allow inbound HTTP traffic for Go backend API"
  }
  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["${data.http.services_public_ip.response_body}/32"]
    description = "Allow inbound SSM - ingress"
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Allow all outbound for Services - egress"
  }
}

resource "aws_network_interface" "raffle-services-eni" {
  subnet_id   = aws_subnet.raffle-services-subnet-public.id
  private_ips = ["172.16.15.100"]

  tags = {
    Name = "${var.raffle_prefix}-services-eni"
  }
}

resource "aws_network_interface_sg_attachment" "raffle-services-attachment" {
  security_group_id    = aws_security_group.raffle-services-sg.id
  network_interface_id = aws_network_interface.raffle-services-eni.id
}

resource "aws_iam_instance_profile" "raffle-services-iam-profile" {
  name = "${var.raffle_prefix}-ec2_profile"
  role = aws_iam_role.raffle-services-iam-role.name
}

resource "aws_iam_role" "raffle-services-iam-role" {
  name               = "${var.raffle_prefix}-ssm-role"
  description        = "The role for the developer resources EC2"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Principal = {
          Service = "ec2.amazonaws.com"
        }
        Effect = "Allow"
        Sid    = ""
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "raffle-services-ssm-policy" {
  role       = aws_iam_role.raffle-services-iam-role.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_instance" "raffle-services-server" {
  ami                  = "ami-028eb925545f314d6"
  instance_type        = "t3.large"
  iam_instance_profile = aws_iam_instance_profile.raffle-services-iam-profile.name

  network_interface {
    network_interface_id = aws_network_interface.raffle-services-eni.id
    device_index         = 0
  }

  user_data = base64encode(<<-EOF
              #!/bin/bash
              sudo yum update -y
              sudo yum install docker -y
              sudo service docker start
              sudo usermod -a -G docker ec2-user
              sudo chkconfig docker on
              
              # Login to Amazon ECR
              aws ecr get-login-password --region ${data.aws_region.current.name} | docker login --username AWS --password-stdin ${data.aws_caller_identity.current.account_id}.dkr.ecr.${data.aws_region.current.name}.amazonaws.com
              
              # Run Go Backend Container mapped to port 8080 with DB environment variables
              sudo docker run -d \
                --name raffle_backend \
                --restart always \
                -p 8080:8080 \
                -e DB_HOST="${aws_db_instance.raffle-test-pgsql.address}" \
                -e DB_PORT="${aws_db_instance.raffle-test-pgsql.port}" \
                -e DB_USER="${aws_db_instance.raffle-test-pgsql.username}" \
                -e DB_PASSWORD="${aws_db_instance.raffle-test-pgsql.password}" \
                -e DB_NAME="raffle" \
                ${data.aws_caller_identity.current.account_id}.dkr.ecr.${data.aws_region.current.name}.amazonaws.com/${var.raffle_prefix}-backend:latest
              EOF
  )

  depends_on = [
    aws_iam_instance_profile.raffle-services-iam-profile,
    aws_iam_role.raffle-services-iam-role,
    aws_iam_role_policy_attachment.raffle-services-ssm-policy
  ]

  tags = {
    Name = "${var.raffle_prefix}-services-ec2"
  }
}

resource "aws_iam_policy" "raffle-services-sds-policy" {
  name = "${var.raffle_prefix}-sds-policy"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "ecr:GetAuthorizationToken",
          "ecr:BatchCheckLayerAvailability",
          "ecr:GetDownloadUrlForLayer",
          "ecr:BatchGetImage",
          "ecr:DescribeImages",
          "logs:CreateLogStream",
          "logs:PutLogEvents",
          "ecs:RunTask",
          "iam:PassRole"
        ]
        Resource = "*"
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "sds-policy-attach" {
  role       = aws_iam_role.raffle-services-iam-role.name
  policy_arn = aws_iam_policy.raffle-services-sds-policy.arn
}