# NixFlow AWS infrastructure

Terraform + Packer to stand up the cloud compute backends NixFlow dispatches to:

- a **HashiCorp Nomad cluster** (Consul service discovery, ACLs bootstrapped on
  first boot) — the primary Nextflow dispatch target;
- **AWS Batch** (spot compute environment + job queue) — an alternative dispatch
  target for Nextflow;
- an **S3 bucket** for Nextflow outputs and the work directory.

Everything shares one dedicated VPC.

## Layout

```
main.tf / variables.tf / outputs.tf / provider.tf   top-level wiring
image.pkr.hcl                                        Packer build of the hashistack-* AMI
post-setup.sh                                        fetch the Nomad user token after apply
shared/                                              Nomad/Consul bootstrap configs + scripts (baked into the AMI)
modules/
  data/     hashistack-* AMI lookup + AZ discovery
  network/  VPC, subnet, IGW, cluster security group
  nomad/    Nomad server + client instances, IAM auto-join, bootstrap tokens
  s3/       Nextflow bucket (terraform-aws-modules/s3-bucket)
  batch/    AWS Batch spot compute env + job queue (terraform-aws-modules/batch)
```

The `nomad` and `batch` modules both attach the single security group from
`network`, and `batch` writes to the bucket from `s3`.

## Prerequisites

- Terraform >= 1.6, Packer >= 1.9, AWS credentials in the environment.
- An existing EC2 key pair in the target region (its name goes in `key_name`).

## Usage

### 1. Build the cluster AMI (Packer)

The Nomad nodes boot from an Ubuntu 22.04 image with Consul/Nomad/Docker baked
in. Build it once per region; rebuild to pick up new versions.

```bash
cd infra/aws
packer init image.pkr.hcl
packer build -var "region=us-east-1" image.pkr.hcl
```

This produces an AMI named `hashistack-<timestamp>`. Terraform's `data` module
auto-selects the most recent `hashistack-*` you own, so you normally don't need
to copy the id. (Set `ami = "ami-..."` in `terraform.tfvars` to pin one.)

### 2. Apply

```bash
cp terraform.tfvars.example terraform.tfvars   # then edit key_name, nextflow_bucket_name, ...
terraform init
terraform plan
terraform apply
```

Servers bootstrap first (~2-3 min), then clients join via Consul cloud auto-join
(the `ConsulAutoJoin` instance tag + the instance-role EC2 read permissions).

### 3. Fetch the Nomad token

ACLs are bootstrapped on first boot and the user token is stashed in Consul KV.
Retrieve it (and delete it from KV) with:

```bash
./post-setup.sh
export NOMAD_ADDR=$(terraform output -raw lb_address_consul_nomad):4646
export NOMAD_TOKEN=$(cat nomad.token)
nomad server members
nomad node status
```

The Nomad UI is at `$(terraform output -raw nomad_addr)/ui`, Consul at
`$(terraform output -raw consul_addr)/ui`.

SSH in with the Ubuntu user: `terraform output -raw ssh_to_server`.

### 4. Point Nextflow at the bucket / queue

```
terraform output nextflow_outdir    # --outdir
terraform output nextflow_workdir   # -work-dir
terraform output batch_job_queue    # process.queue (Batch executor)
terraform output batch_region       # aws.region
```

### 5. Tear down

```bash
terraform destroy
```

`nextflow_bucket_force_destroy` defaults to `true`, so the bucket is wiped on
destroy even if Nextflow left objects behind.

## Notes / trade-offs

- Single public subnet in one AZ — fine for this workload, not production-shaped.
- One security group is shared by Nomad nodes and Batch instances. The operator
  CIDRs (`ssh_cidr`, `allowlist_ip`) default to `0.0.0.0/0`; lock them down.
- Local Terraform state by default; a commented S3 backend is in `provider.tf`.
- SSH user is `ubuntu` (hashistack AMI), not `root`.
