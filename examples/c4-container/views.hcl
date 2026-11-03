view "main" {
  title = "Internet Banking System: containers"
  edges = [edge_kind.uses]
}

# The same view with a partial layout file: some nodes pinned, the rest placed by the spiral
# placer around them.
view "pinned" {
  title = "Internet Banking System: containers, partly arranged"
  edges = [edge_kind.uses]
}

# A dynamic diagram: the containers a sign-in passes through, with each interaction drawn
# animated from caller to callee.
view "sign-in" {
  title   = "Internet Banking System: signing in"
  include = [node.customer, node.banking, node.spa, node.api, node.database]
  edges   = [edge_kind.interaction]
}
