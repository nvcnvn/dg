view "main" {
  title = "Shapes and styles"
}

# Wiring with pinned nodes and ports and with edge hints: sides, a via point and a label
# position.
view "wiring" {
  title   = "Ports and hints"
  include = [node.rack, node.chip, node.lab, descendants.lab, node.point, node.rounded, node.diamond, node.bullseye, node.cylinder]
}

# The shapes and node styles in a grid, pinned by its layout file.
view "gallery" {
  title   = "Shape gallery"
  include = [node.point, node.rounded, node.diamond, node.bullseye, node.rect, node.stadium, node.ellipse, node.cylinder, node.hexagon, node.parallelogram, node.document, node.person, node.cloud, node.text, node.image, node.table, node.external, node.locked, node.bold, node.faded]
}
