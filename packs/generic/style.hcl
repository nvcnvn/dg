# style "node" and edge_style "default" set every property that has a default (spec 8.4).

style "node" {
  shape         = "rect"
  fill          = "#FFFFFF"
  stroke        = "#3C4043"
  stroke_width  = 1
  stroke_dash   = []
  corner_radius = 8
  opacity       = 1
  text_color    = "#202124"
  font_size     = 14
  font_weight   = 400
  padding       = 10
  min_w         = 100
  min_h         = 40
  max_w         = 240
  z             = 0
  layout        = "free"
  direction     = "vertical"
  gap           = 10
  stretch       = false
  columns       = 3
  align         = "start"
  label_side    = "top"
}

style "service" {
  shape  = "rounded"
  fill   = "#E8F0FE"
  stroke = "#1A73E8"
}

style "database" {
  shape  = "cylinder"
  fill   = "#FEF7E0"
  stroke = "#E37400"
}

style "queue" {
  shape  = "parallelogram"
  fill   = "#F3E8FD"
  stroke = "#9334E6"
}

style "storage" {
  shape  = "cylinder"
  fill   = "#FEF7E0"
  stroke = "#E37400"
}

style "person" {
  shape  = "person"
  fill   = "#E8F0FE"
  stroke = "#1A73E8"
}

style "external" {
  shape       = "rounded"
  fill        = "#F1F3F4"
  stroke      = "#5F6368"
  stroke_dash = [6, 4]
}

style "note" {
  shape  = "document"
  fill   = "#FEF7E0"
  stroke = "#B06000"
}

style "decision" {
  shape = "diamond"
}

style "terminal" {
  shape = "stadium"
}

style "initial" {
  shape = "point"
  fill  = "#202124"
  min_w = 20
  min_h = 20
}

style "final" {
  shape = "bullseye"
  fill  = "#202124"
  min_w = 26
  min_h = 26
}

style "junction" {
  shape = "point"
  fill  = "#3C4043"
  min_w = 10
  min_h = 10
}

style "group" {
  fill    = "#F8F9FA"
  stroke  = "#9AA0A6"
  padding = 20
}

style "lane-set" {
  fill      = "#FFFFFF"
  stroke    = "#9AA0A6"
  padding   = 0
  layout    = "stack"
  direction = "vertical"
  gap       = 0
  stretch   = true
}

style "lane" {
  fill       = "#F8F9FA"
  stroke     = "#9AA0A6"
  padding    = 20
  label_side = "left"
}

style "grid-group" {
  fill    = "#F8F9FA"
  stroke  = "#9AA0A6"
  padding = 20
  layout  = "grid"
  columns = 3
}

style "state" {
  shape   = "rounded"
  fill    = "#FFFFFF"
  stroke  = "#3C4043"
  padding = 20
}

style "table" {
  shape   = "table"
  padding = 8
}

style "class" {
  shape   = "table"
  padding = 8
}

edge_style "default" {
  stroke       = "#3C4043"
  stroke_width = 1.5
  stroke_dash  = []
  arrow_start  = "none"
  arrow_end    = "arrow"
  routing      = "orthogonal"
  font_size    = 12
  animate      = "none"
}

edge_style "async" {
  stroke_dash = [6, 4]
}

edge_style "association" {
  arrow_end = "none"
}

edge_style "inherits" {
  arrow_end = "triangle-open"
}

edge_style "realizes" {
  stroke_dash = [6, 4]
  arrow_end   = "triangle-open"
}

edge_style "composition" {
  arrow_start = "diamond"
  arrow_end   = "none"
}

edge_style "aggregation" {
  arrow_start = "diamond-open"
  arrow_end   = "none"
}

edge_style "dependency" {
  stroke_dash = [6, 4]
}

edge_style "tag.start-one" {
  arrow_start = "crow-one"
}

edge_style "tag.start-zero-one" {
  arrow_start = "crow-zero-one"
}

edge_style "tag.start-many" {
  arrow_start = "crow-many"
}

edge_style "tag.start-one-many" {
  arrow_start = "crow-one-many"
}

edge_style "tag.start-zero-many" {
  arrow_start = "crow-zero-many"
}

edge_style "tag.end-one" {
  arrow_end = "crow-one"
}

edge_style "tag.end-zero-one" {
  arrow_end = "crow-zero-one"
}

edge_style "tag.end-many" {
  arrow_end = "crow-many"
}

edge_style "tag.end-one-many" {
  arrow_end = "crow-one-many"
}

edge_style "tag.end-zero-many" {
  arrow_end = "crow-zero-many"
}

overlay_style "hull" {
  fill         = "none"
  stroke       = "#5F6368"
  stroke_width = 1.5
  stroke_dash  = [4, 4]
  opacity      = 1
  padding      = 12
}
