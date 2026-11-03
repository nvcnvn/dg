# Generated; read-only.

node "ext" {
  kind = "external"
}

edge "ext-gw" {
  from = node.ext
  to   = node.gateway
}
