view "main" {
  theme = "dark"

  overlay "o" {
    members = [all]
    colour  = "red"
  }

  node "a" {
    label = "A"
    color = "red"
  }

  style "tag.x" {
    color = "red"
  }

  edge_style "tag.x" {
    color = "red"
  }
}
