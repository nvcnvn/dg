view "main" {
  title = "Everything"
}

view "fabric" {
  include = [kind.switch, tag.generated]
  edges   = [edge_kind.default, tag.hot]
}
