node "a" {}

node "b" {}

edge "both" {
  from   = node.a
  to     = node.b
  label  = "x"
  labels = { mid = "y" }
}

edge "where" {
  from   = node.a
  to     = node.b
  labels = { start = "1", middle = "places", end = "0..*" }
}
