# A queue for this experiment's requests
resource "aws_sqs_queue" "requests" {
  name = "${var.name}-requests"
}

# Allow queue to receive messages from requests topic
resource "aws_sqs_queue_policy" "requests" {
  queue_url = aws_sqs_queue.requests.id

  policy = <<POLICY
{
  "Version": "2012-10-17",
  "Id": "sqspolicy",
  "Statement": [
    {
      "Sid": "First",
      "Effect": "Allow",
      "Principal": "*",
      "Action": "sqs:SendMessage",
      "Resource": "${aws_sqs_queue.requests.arn}",
      "Condition": {
        "ArnEquals": {
          "aws:SourceArn": "${var.request_sns_topic_arn}"
        }
      }
    }
  ]
}
POLICY
}

# Subscribe queue to requests topic
resource "aws_sns_topic_subscription" "requests_sqs_target" {
  topic_arn = var.request_sns_topic_arn
  protocol  = "sqs"
  endpoint  = aws_sqs_queue.requests.arn
}

