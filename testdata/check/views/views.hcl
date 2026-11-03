view "main" {
  include  = [all, node.nope, kind.nope, tag.unused, children.a, descendants.nope, edge.ab]
  exclude  = [node["g"], tag.critical]
  collapse = [node.a, kind.node, kind.group]
  edges    = [edge_kind.default, edge_kind.nope, edge.nope, tag.none]
  overlay "o" {
    members = [tag.critical]
    style   = "ghost"
  }
  overlay "p" {
    members = [node.g]
    style   = "hull"
  }
  style "node.nope" {
    fill = "#FFF"
  }
  style "node.a" {
    shape = "blob"
  }
  edge_style "edge.ab" {
    stroke = "#000"
  }
  edge_style "edge.zz" {}
}

view "inside" {
  of = node.a
}

view "drill" {
  of = node.g
}

view "cold" {
  include = [tag.cold]
}

view "bad" {
  include = [node, all.x, foo.bar, "a"]
  of      = kind.group
}

view "sideways" {
  direction = "sideways"
}
