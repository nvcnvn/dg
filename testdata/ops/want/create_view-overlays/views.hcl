view "main" { include = [all] }

view "pool" {
  title = "Inside the pool"
  of    = node.pool
  include = [
    descendants.pool, # the workers
    node["worker-2"], # the spare
    node.gateway,     # the caller
  ]
}

# Calls from the gateway.
view "calls" {
  include = [ node.gateway,node["worker-1"], node.pool ]
  edges   = [edge.gw-w1] /* only the call */

  style "node.gateway" {
    fill = "#FFEEEE"
  }
}

view "solo" { include = [node.gateway] }

view "listed" {
  include = [
    node.gateway, # first
    node.cache,   # second
    node.pool,
    node["worker-1"],
  ]
}

view "incident" {
  direction = "right"
  collapse  = [node.rack]
  overlay "blast" {
    members = [tag.edge, node["worker-1"]]
    style   = "hull"
    label   = "Blast radius"
  }
  style "tag.edge" {
    fill = "#FCE8E6"
  }
}
