view "services" {
  title   = "Request path"
  include = [kind.service, kind.group, kind.gpu-node]
  edges   = [edge_kind.default]
}

# Edges end at ports, which are drawn from M3; until then dg export refuses this view.
view "rdma" {
  title   = "RDMA fabric"
  include = [kind.gpu-node, kind.switch, kind.storage]
  edges   = [edge_kind.rdma]
}
