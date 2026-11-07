kind "vpc" {
  extends  = "group"
  category = "Networking"
}

kind "lambda" {
  extends  = "service"
  category = "Compute"
}

kind "edge-lambda" {
  extends = "lambda"
}

edge_kind "peering" {}
