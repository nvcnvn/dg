kind "node" {}

kind "service" {
  extends = "node"
}

kind "group" {
  extends   = "node"
  container = true
}

edge_kind "default" {}
