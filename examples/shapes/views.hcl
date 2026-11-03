view "main" {
  title = "Shapes and styles"
}

# Wiring with pinned nodes and ports and with edge hints: sides, a via point and a label
# position.
view "wiring" {
  title   = "Ports and hints"
  include = [node.rack, node.chip, node.lab, descendants.lab, node.point, node.rounded, node.diamond, node.bullseye, node.cylinder]
}
