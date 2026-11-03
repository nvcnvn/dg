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

style "external-system" {
  fill   = "#999999"
  stroke = "#8A8A8A"
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

style "boundary" {
  shape       = "rect"
  fill        = "none"
  stroke      = "#444444"
  stroke_dash = [8, 4]
  text_color  = "#444444"
  padding     = 20
}

edge_style "uses" {
  stroke      = "#707070"
  stroke_dash = [6, 4]
  font_size   = 11
}
