kind "node" {}

kind "service" {}
kind "database" {}
kind "queue" {}
kind "storage" {}
kind "person" {}
kind "external" {}
kind "note" {}
kind "decision" {}
kind "terminal" {}
kind "initial" {}
kind "final" {}
kind "simple-state" {}
kind "junction" {}

kind "group" {
  container = true
}

kind "lane-set" {
  container = true
}

kind "lane" {
  container = true
}

kind "grid-group" {
  container = true
}

kind "state" {
  container = true
}

kind "table" {
  rows = true
}

kind "class" {
  rows = true
}

edge_kind "default" {}
edge_kind "async" {}
edge_kind "association" {}
edge_kind "inherits" {}
edge_kind "realizes" {}
edge_kind "composition" {}
edge_kind "aggregation" {}
edge_kind "dependency" {}
