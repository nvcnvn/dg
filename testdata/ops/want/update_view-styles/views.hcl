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

view "calls" {
  include = [ node.gateway,node["worker-1"], node.pool ]
  edges   = [edge.gw-w1] /* only the call */

  style "node.gateway" {
    fill = "#FFEEEE"
  }
  style "tag.edge" {
    stroke_dash  = [4, 2]
    stroke_width = 2
  }
  edge_style "edge.gw-w1" {
    animate = "flow"
    stroke  = "#D93025"
  }
}

view "solo" { include = [node.gateway] }
