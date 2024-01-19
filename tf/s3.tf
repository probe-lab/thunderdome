module "s3_bucket_public" {
  source  = "terraform-aws-modules/s3-bucket/aws"
  version = "4.0.1"

  bucket             = "pl-thunderdome-public"
  acl                = "public-read"
  block_public_acls  = false
  ignore_public_acls = false

  force_destroy = true
  versioning = {
    enabled = true
  }
}

resource "aws_s3_bucket" "s3_bucket_private" {
  bucket        = "pl-thunderdome-private"
  force_destroy = true
}

resource "aws_s3_bucket_acl" "s3_bucket_private" {
  bucket = aws_s3_bucket.s3_bucket_private.id
  acl    = "private"
}

resource "aws_s3_object" "infra_json" {
  bucket  = aws_s3_bucket.s3_bucket_private.id
  key     = "infra.json"
  content = local.infra_json
}
