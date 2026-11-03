# One lifted edge keeps its labels and style; its port is dropped. Edges inside api are hidden.
view "one" {
  collapse  = [node.api]
  direction = "right"
}

# Lifted edges of one kind merge into one labeled ×N, styled by that kind, keeping the port
# their ends that are not lifted share. The direct edge to the collapsed container stays apart.
view "same-kind" {
  collapse = [node.sys]
  edges    = [edge_kind.async, edge.client-sys]
}

# Lifted edges of different kinds merge into one styled by default, without the port only some
# of them start at; a lift: hint applies.
view "mixed" {
  collapse = [node.sys]
}

# A hidden end is lifted to the outermost collapsed container holding it.
view "nested" {
  collapse = [node.api, node.sys]
}

# A collapsed container that is not visible draws no edge to what it hides.
view "excluded" {
  exclude  = [node.api]
  collapse = [node.api]
}

# An overlay member hidden by a collapse is its collapsed container; an overlay without drawn
# members is not drawn.
view "overlay" {
  exclude  = [node.queue]
  collapse = [node.api]

  overlay "path" {
    members = [node.client, node.handler]
    label   = "Request path"
  }

  overlay "gone" {
    members = [node.queue]
  }
}

# A lifted edge to a container with a quoted ID: its key encodes %, : and #, and its hint applies.
view "quoted" {
  include  = [node.client, node["rack:a%b#c"], node.disk]
  collapse = [node["rack:a%b#c"]]
}
