resource "aws_vpc" "this" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = { Name = "nomad-vpc" }
}

resource "aws_internet_gateway" "this" {
  vpc_id = aws_vpc.this.id

  tags = { Name = "nomad-igw" }
}

resource "aws_subnet" "public" {
  vpc_id                  = aws_vpc.this.id
  cidr_block              = var.subnet_cidr
  availability_zone       = var.az
  map_public_ip_on_launch = true

  tags = { Name = "nomad-public" }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.this.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.this.id
  }

  tags = { Name = "nomad-public-rt" }
}

resource "aws_route_table_association" "public" {
  subnet_id      = aws_subnet.public.id
  route_table_id = aws_route_table.public.id
}

resource "aws_security_group" "cluster" {
  name        = "nomad-cluster"
  description = "SSH + Nomad/Consul UI from operator, unrestricted intra-cluster traffic"
  vpc_id      = aws_vpc.this.id

  tags = { Name = "nomad-cluster" }
}

resource "aws_vpc_security_group_ingress_rule" "ssh" {
  security_group_id = aws_security_group.cluster.id
  description       = "SSH from operator CIDR"
  cidr_ipv4         = var.ssh_cidr
  from_port         = 22
  to_port           = 22
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_ingress_rule" "nomad_ui" {
  security_group_id = aws_security_group.cluster.id
  description       = "Nomad HTTP API / UI from operator CIDR"
  cidr_ipv4         = var.allowlist_ip
  from_port         = 4646
  to_port           = 4646
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_ingress_rule" "consul_ui" {
  security_group_id = aws_security_group.cluster.id
  description       = "Consul HTTP API / UI from operator CIDR"
  cidr_ipv4         = var.allowlist_ip
  from_port         = 8500
  to_port           = 8500
  ip_protocol       = "tcp"
}

# Self-referential rule: every instance in this SG can reach every other on any
# port. Covers Nomad RPC (4647), Serf (4648), Consul (8300-8302/8500/8502/8600),
# and AWS Batch instances without enumerating ports here.
resource "aws_vpc_security_group_ingress_rule" "intra_cluster" {
  security_group_id            = aws_security_group.cluster.id
  description                  = "All traffic between cluster members"
  referenced_security_group_id = aws_security_group.cluster.id
  ip_protocol                  = "-1"
}

resource "aws_vpc_security_group_egress_rule" "all" {
  security_group_id = aws_security_group.cluster.id
  description       = "Allow all egress"
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "-1"
}
