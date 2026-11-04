view "main" {
  include = [all]
  node "store" {
    label = "One"
  }
}

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
    node.store,   # second
    node.pool,
    node["worker-1"],
  ]
}
