node "x" {}

edge "e" {
  from = node.x
  to   = node.x
}

node "all" {}

node "a\tb" {}

node "" {}

edge "lift" {
  from = node.x
  to   = node["kube-system/coredns"]
}

node "kube-system/coredns" {
  tags = ["ok", "not ok"]
}
