locals {
  # text/x-shellscript MIME part following Seqera's "AWS Batch without Fusion v2" guidance.
  #
  # Why shellscript instead of cloud-config write_files+runcmd:
  #   cloud-config runcmd runs after systemd services have been activated; stopping docker at
  #   that point briefly drops network connectivity, silently breaking the subsequent curl.
  #   text/x-shellscript executes during the early cloud-init phase — before docker/ECS start —
  #   so ECS can be configured in place and no docker stop/restart is needed.
  #
  # Why miniconda awscli instead of the standalone awscli v2 binary:
  #   The v2 binary dynamically links libz.so.1.  When Nextflow mounts it into task containers
  #   via aws.batch.cliPath, containers that lack zlib (e.g. minimal bioconda images) fail.
  #   The miniconda tarball ships its own Python interpreter, so the mounted directory is
  #   fully self-contained regardless of the container image.
  user_data = <<-EOT
    MIME-Version: 1.0
    Content-Type: multipart/mixed; boundary="==BOUNDARY=="

    --==BOUNDARY==
    Content-Type: text/x-shellscript; charset="us-ascii"

    #!/bin/bash
    exec > >(tee /var/log/custom-ce.log | logger -t custom-ce -s 2>/dev/console) 2>&1

    yum install -q -y jq unzip wget

    ## ECS configuration — written before ECS agent starts (no docker stop/restart needed here)
    mkdir -p /etc/ecs
    echo ECS_IMAGE_PULL_BEHAVIOR=once                   >> /etc/ecs/ecs.config
    echo ECS_ENABLE_AWSLOGS_EXECUTIONROLE_OVERRIDE=true >> /etc/ecs/ecs.config
    echo ECS_ENABLE_SPOT_INSTANCE_DRAINING=true         >> /etc/ecs/ecs.config
    echo ECS_CONTAINER_CREATE_TIMEOUT=10m               >> /etc/ecs/ecs.config
    echo ECS_CONTAINER_START_TIMEOUT=10m                >> /etc/ecs/ecs.config
    echo ECS_CONTAINER_STOP_TIMEOUT=10m                 >> /etc/ecs/ecs.config
    echo ECS_MANIFEST_PULL_TIMEOUT=10m                  >> /etc/ecs/ecs.config

    ## Install self-contained miniconda awscli (avoids libz.so.1 dep of standalone v2 binary).
    ## -fsSL: fail on HTTP errors, silent progress, show errors, follow redirects.
    curl -fsSL https://nf-xpack.seqera.io/miniconda-awscli/miniconda-25.3.1-awscli-1.40.12.tar.gz \
      | tar xz -C /
    if [ -f /home/ec2-user/miniconda/bin/aws ]; then
      ln -sf /home/ec2-user/miniconda/bin/aws /usr/bin/aws
      echo "INFO: awscli OK — $(/home/ec2-user/miniconda/bin/aws --version 2>&1)"
    else
      echo "ERROR: /home/ec2-user/miniconda/bin/aws not found after extraction" >&2
      exit 1
    fi

    ## Kernel settings to prevent OOM under heavy S3 I/O
    echo "1258291200" > /proc/sys/vm/dirty_bytes
    echo "629145600"  > /proc/sys/vm/dirty_background_bytes

    --==BOUNDARY==--
  EOT
}

resource "aws_launch_template" "this" {
  name_prefix = "${var.name_prefix}-batch-"
  description = "Seqera-compatible CE template: installs miniconda awscli and configures ECS for Nextflow on AWS Batch"

  user_data = base64encode(local.user_data)

  tag_specifications {
    resource_type = "instance"
    tags          = merge(var.tags, { Name = "${var.name_prefix}-batch-spot" })
  }

  tags = var.tags
}

resource "aws_iam_policy" "s3_access" {
  name        = "${var.name_prefix}-batch-s3-access"
  description = "Grants Nextflow Batch jobs read/write access to the workflow bucket"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:ListBucket",
          "s3:GetBucketLocation",
        ]
        Resource = var.bucket_arn
      },
      {
        Effect = "Allow"
        Action = [
          "s3:GetObject",
          "s3:PutObject",
          "s3:DeleteObject",
          "s3:AbortMultipartUpload",
          "s3:ListMultipartUploadParts",
        ]
        Resource = "${var.bucket_arn}/*"
      },
    ]
  })

  tags = var.tags
}

# Permissions Nextflow needs on the compute instance to submit and manage child
# Batch jobs, query ECS/EC2 metadata, and write CloudWatch logs.
resource "aws_iam_policy" "batch_ops" {
  name        = "${var.name_prefix}-batch-ops"
  description = "Batch/ECS/EC2/CloudWatch permissions required by Nextflow on compute instances"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "BatchJobManagement"
        Effect = "Allow"
        Action = [
          "batch:DescribeJobQueues",
          "batch:CancelJob",
          "batch:SubmitJob",
          "batch:ListJobs",
          "batch:DescribeComputeEnvironments",
          "batch:TerminateJob",
          "batch:DescribeJobs",
          "batch:RegisterJobDefinition",
          "batch:DescribeJobDefinitions",
          "batch:TagResource",
          "ecs:DescribeTasks",
          "ecs:DescribeContainerInstances",
          "ec2:DescribeInstances",
          "ec2:DescribeInstanceTypes",
          "ec2:DescribeInstanceAttribute",
          "ec2:DescribeInstanceStatus",
        ]
        Resource = "*"
      },
      {
        Sid    = "CloudWatchLogs"
        Effect = "Allow"
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:DescribeLogGroups",
          "logs:DescribeLogStreams",
          "logs:FilterLogEvents",
          "logs:GetLogEvents",
          "logs:ListTagsLogGroup",
          "logs:PutLogEvents",
          "logs:StartQuery",
          "logs:StopQuery",
          "logs:TestMetricFilter",
        ]
        Resource = "*"
      },
    ]
  })

  tags = var.tags
}

module "batch" {
  source  = "terraform-aws-modules/batch/aws"
  version = "~> 3.0"

  instance_iam_role_name        = "${var.name_prefix}-batch-instance"
  instance_iam_role_description = "ECS instance role for Nextflow on AWS Batch"
  instance_iam_role_additional_policies = {
    S3Access = aws_iam_policy.s3_access.arn
    BatchOps = aws_iam_policy.batch_ops.arn
  }

  service_iam_role_name = "${var.name_prefix}-batch-service"

  create_spot_fleet_iam_role = true
  spot_fleet_iam_role_name   = "${var.name_prefix}-batch-spot-fleet"

  compute_environments = {
    spot = {
      name_prefix = "${var.name_prefix}-spot-"

      compute_resources = {
        type                = "SPOT"
        allocation_strategy = "SPOT_CAPACITY_OPTIMIZED"
        bid_percentage      = var.spot_bid_percentage

        min_vcpus     = 0
        max_vcpus     = var.max_vcpus
        desired_vcpus = 0

        instance_types = var.instance_types

        subnets            = var.subnet_ids
        security_group_ids = var.security_group_ids

        launch_template = {
          launch_template_id = aws_launch_template.this.id
          version            = "$Latest"
        }

        tags = {
          Name = "${var.name_prefix}-batch-spot"
        }
      }
    }
  }

  # Job queue is created below with a standalone resource to avoid the
  # deprecation warning emitted by the module's built-in queue output under
  # aws provider v6.
  job_queues = {}

  tags = var.tags
}

resource "aws_batch_job_queue" "nextflow" {
  name     = "${var.name_prefix}-nextflow"
  state    = "ENABLED"
  priority = 1

  compute_environment_order {
    order               = 0
    compute_environment = module.batch.compute_environments["spot"].arn
  }

  tags = var.tags
}
