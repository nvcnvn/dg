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

view "solo" { include = [node.gateway] }

view "listed" {
  include = [
    node.gateway, # first
    node.cache,   # second
    node.pool,
    node["worker-1"],
  ]
}
