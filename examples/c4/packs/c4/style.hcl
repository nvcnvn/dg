# Elements are drawn as the boxes of a diagram that does not show their inside; a view that
# shows the inside of one draws it as a boundary with a view style rule.

style "c4-person" {
  fill       = "#08427B"
  stroke     = "#073B6F"
  text_color = "#FFFFFF"
  max_w      = 220
}

style "system" {
  shape      = "rounded"
  fill       = "#1168BD"
  stroke     = "#0B4884"
  text_color = "#FFFFFF"
  min_w      = 200
  max_w      = 240
  padding    = 12
}

style "tag.existing" {
  fill   = "#999999"
  stroke = "#8A8A8A"
}

style "external-system" {
  shape      = "rounded"
  fill       = "#999999"
  stroke     = "#8A8A8A"
  text_color = "#FFFFFF"
  min_w      = 200
  max_w      = 240
  padding    = 12
}

style "app" {
  shape      = "rounded"
  fill       = "#438DD5"
  stroke     = "#3C7FC0"
  text_color = "#FFFFFF"
  min_w      = 200
  max_w      = 240
  padding    = 12
}

style "store" {
  shape      = "cylinder"
  fill       = "#438DD5"
  stroke     = "#3C7FC0"
  text_color = "#FFFFFF"
  min_w      = 200
  max_w      = 240
  padding    = 12
}

style "component" {
  shape      = "rounded"
  fill       = "#85BBF0"
  stroke     = "#78A8D8"
  text_color = "#000000"
  min_w      = 200
  max_w      = 240
  padding    = 12
}

edge_style "uses" {
  stroke      = "#707070"
  stroke_dash = [6, 4]
  font_size   = 11
}
