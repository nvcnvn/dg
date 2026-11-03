node "g" {
  kind = "group"
}

node "a" {
  kind   = "nope"
  parent = node.g
}

node "b" {
  parent = node.a.port.p
}

node "c" {
  parent = node.missing
}

node "d" {
  parent = node.c
}

node "loop1" {
  kind   = "group"
  parent = node.loop2
}

node "loop2" {
  kind   = "group"
  parent = node.loop1
}

node "t" {
  kind  = "table"
  ports = ["p1"]
  row "id" {}
  row "save()" {}
}

edge "ok" {
  from = node.t.port.p1
  to   = node.t.row["save()"]
}

edge "bad-ends" {
  from = node.t.port.nope
  to   = node.t.row.nope
}

edge "undefined" {
  from = node.missing
  to   = node["missing/too"]
  kind = "nope"
}

edge "syntax" {
  from = node.t.column.id
  to   = edge.ok
}

edge "half" {
  from = node.g
}
