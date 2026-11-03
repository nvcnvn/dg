node "g" {
  kind = "group"
}

node "a" {
  parent = node.g
  tags   = ["critical"]
}

node "b" {}

edge "ab" {
  from = node.a
  to   = node.b
  tags = ["cold"]
}
