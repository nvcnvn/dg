node "x" {}

edge "e" {
  from = node.x
  to   = node.x
}

edge "lift:x:x" {
  from = node.x
  to   = node.x
}
