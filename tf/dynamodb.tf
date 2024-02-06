resource "aws_dynamodb_table" "experiments" {
  name         = "experiments"
  billing_mode = "PROVISIONED"
  hash_key     = "name"

  read_capacity  = 1
  write_capacity = 1

  # name of the experiment
  attribute {
    name = "name"
    type = "S"
  }
}
