shape "rack" {
  svg      = file("shapes/rack.svg")
  text_box = [0.05, 0.2, 0.9, 0.75]
  anchors  = { up = [0.5, 0], down = [0.5, 1] }
  stretch  = "nine-slice"
}

shape "chip" {
  svg      = file("shapes/chip.svg")
  text_box = [0.15, 0.15, 0.7, 0.7]
  anchors  = { in = [0, 0.5], out = [1, 0.5] }
  stretch  = "uniform"
}

style "rack" {
  shape = "rack"
}

style "chip" {
  shape = "chip"
}

style "picture" {
  shape = "image"
  image = file("icons/photo.svg")
}

style "tag.rect" {
  shape = "rect"
}

style "tag.ellipse" {
  shape = "ellipse"
}

style "tag.hexagon" {
  shape = "hexagon"
}

style "tag.cloud" {
  shape  = "cloud"
  fill   = "#E8F0FE"
  stroke = "#1A73E8"
}

style "tag.text" {
  shape = "text"
}

style "tag.locked" {
  icon = file("icons/lock.svg")
}

style "tag.bold" {
  font_weight  = 700
  font_size    = 16
  stroke_width = 3
}

style "tag.faded" {
  opacity     = 0.5
  stroke_dash = [4, 2]
}

style "tag.on-top" {
  z          = 1
  fill       = "#FCE8E6"
  text_color = "#A50E0E"
}

edge_style "tag.polyline" {
  routing = "polyline"
}

edge_style "tag.straight" {
  routing = "straight"
}

edge_style "tag.both" {
  arrow_start  = "arrow"
  stroke_width = 2.5
}
