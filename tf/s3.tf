module "s3_bucket_public" {
  source = "terraform-aws-modules/s3-bucket/aws"

  bucket        = "pl-thunderdome-public"
  acl           = "public-read"
  force_destroy = true
  versioning = {
    enabled = true
  }
}
