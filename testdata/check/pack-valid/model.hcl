node "net" {
  kind = "aws-vpc"
}

node "fn" {
  kind   = "aws-lambda"
  parent = node.net
  tags   = ["legacy"]
}

node "fn2" {
  kind   = "aws-edge-lambda"
  parent = node.net
}

edge "e" {
  from = node.fn
  to   = node.fn2
  kind = "aws-peering"
}
