# Consul/Nomad bootstrap token. nomad_id is the token accessor, nomad_token the
# secret. Both are baked into the user-data scripts so servers and clients share
# the same auto-join token and the operator can later read it back from Consul KV
# (see post-setup.sh).
resource "random_uuid" "nomad_id" {}

resource "random_uuid" "nomad_token" {}

resource "aws_instance" "server" {
  count = var.server_count

  ami                    = var.ami_id
  instance_type          = var.server_instance_type
  subnet_id              = var.subnet_id
  vpc_security_group_ids = var.security_group_ids
  key_name               = var.key_name
  iam_instance_profile   = aws_iam_instance_profile.instance_profile.name

  # ConsulAutoJoin is load-bearing: Consul's cloud auto-join discovers peers by
  # this tag (see var.retry_join), so it must live on the instance itself.
  tags = {
    Name           = "${var.name_prefix}-server-${count.index}"
    ConsulAutoJoin = "auto-join"
    NomadType      = "server"
  }

  root_block_device {
    volume_type           = "gp3"
    volume_size           = var.root_volume_size_gb
    delete_on_termination = true
  }

  user_data = templatefile("${path.module}/../../shared/data-scripts/user-data-server.sh", {
    server_count              = var.server_count
    region                    = var.region
    cloud_env                 = "aws"
    retry_join                = var.retry_join
    nomad_binary              = var.nomad_binary
    nomad_consul_token_id     = random_uuid.nomad_id.result
    nomad_consul_token_secret = random_uuid.nomad_token.result
  })

  # Auto-join reads the ConsulAutoJoin tag from instance metadata (IMDS).
  metadata_options {
    http_endpoint          = "enabled"
    instance_metadata_tags = "enabled"
  }
}

resource "aws_instance" "client" {
  count = var.client_count

  ami                    = var.ami_id
  instance_type          = var.client_instance_type
  subnet_id              = var.subnet_id
  vpc_security_group_ids = var.security_group_ids
  key_name               = var.key_name
  iam_instance_profile   = aws_iam_instance_profile.instance_profile.name
  depends_on             = [aws_instance.server]

  tags = {
    Name           = "${var.name_prefix}-client-${count.index}"
    ConsulAutoJoin = "auto-join"
    NomadType      = "client"
  }

  root_block_device {
    volume_type           = "gp3"
    volume_size           = var.root_volume_size_gb
    delete_on_termination = true
  }

  # Extra raw volume for job scratch / Docker data. Not formatted by the
  # bootstrap scripts; jobs that need it claim it.
  ebs_block_device {
    device_name           = "/dev/xvdd"
    volume_type           = "gp3"
    volume_size           = var.client_data_volume_size_gb
    delete_on_termination = true
  }

  # The client template intentionally takes no server_count.
  user_data = templatefile("${path.module}/../../shared/data-scripts/user-data-client.sh", {
    region                    = var.region
    cloud_env                 = "aws"
    retry_join                = var.retry_join
    nomad_binary              = var.nomad_binary
    nomad_consul_token_id     = random_uuid.nomad_id.result
    nomad_consul_token_secret = random_uuid.nomad_token.result
  })

  metadata_options {
    http_endpoint          = "enabled"
    instance_metadata_tags = "enabled"
  }
}

# ----- IAM: instance profile granting Consul cloud auto-join -----

resource "aws_iam_instance_profile" "instance_profile" {
  name_prefix = var.name_prefix
  role        = aws_iam_role.instance_role.name
}

resource "aws_iam_role" "instance_role" {
  name_prefix        = var.name_prefix
  assume_role_policy = data.aws_iam_policy_document.instance_role.json
}

data "aws_iam_policy_document" "instance_role" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role_policy" "auto_discover_cluster" {
  name   = "${var.name_prefix}-auto-discover-cluster"
  role   = aws_iam_role.instance_role.id
  policy = data.aws_iam_policy_document.auto_discover_cluster.json
}

data "aws_iam_policy_document" "auto_discover_cluster" {
  statement {
    effect = "Allow"

    actions = [
      "ec2:DescribeInstances",
      "ec2:DescribeTags",
      "autoscaling:DescribeAutoScalingGroups",
    ]

    resources = ["*"]
  }
}
