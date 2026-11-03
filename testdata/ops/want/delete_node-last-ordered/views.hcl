view "main" { include = [all] }

view "pool" {
  title = "Inside the pool"
  of    = node.pool
  include = [
    descendants.pool, # the workers
    node.gateway,     # the caller
  ]
}

view "calls" {
  include = [ node.gateway,node["worker-1"], node.pool ]
  edges   = [edge.gw-w1] /* only the call */

  style "node.gateway" {
    fill = "#FFEEEE"
  }
}

view "solo" { include = [node.gateway] }
