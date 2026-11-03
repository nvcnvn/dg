node "g" {
  kind = "group"
}

node "lanes" {
  kind = "lane-set"
}

node "a" {
  parent = node.g
  ports  = ["p1"]
}

node "b" {
  parent = node.lanes
}

node "sys:ctl" {}

edge "ab" {
  from = node.a
  to   = node.b
}
