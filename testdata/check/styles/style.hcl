style "node" {
  shape = "blob"
  fill  = "red"
}

style "service" {
  stroke_width = -1
  stroke_dash  = [4, "x"]
  opacity      = 2
  font_weight  = 450
  z            = 1.5
  layout       = "flow"
  columns      = 0
  stretch      = "yes"
  colour       = "#FFF"
}

style "service" {}

style "tag.bad tag" {}

style "node.api" {}

style "ghost" {}

style "a.b" {}

style "gpu" {
  shape = "gpu"
}

edge_style "default" {
  fill      = "#FFF"
  arrow_end = "spear"
  routing   = "curvy"
  animate   = "flow"
}

edge_style "edge.e" {}

overlay_style "bad name" {
  padding = -3
}

shape "gpu" {
  svg      = file("shapes/gpu.svg")
  text_box = [0.1, 0.6, 0.8]
  anchors  = { nic0 = [1, 0.3], nic9 = [1, 0.7] }
  stretch  = "nine-slice"
}

shape "rect" {
  svg = file("../outside.svg")
}

shape "missing" {
  svg = file("shapes/missing.svg")
}

style "tag.broken" {
  icon = file("shapes/broken.svg")
}

shape "page" {
  svg = file("shapes/page.svg")
}
