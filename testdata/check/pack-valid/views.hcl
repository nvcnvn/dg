view "main" {
  include = [kind.aws-vpc, kind.aws-lambda, kind.aws-edge-lambda]
  overlay "o" {
    members = [node.fn]
    style   = "aws-zone"
  }
}
