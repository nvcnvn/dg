shape "rack" {
  svg      = file("shapes/rack.svg")
  text_box = [0.05, 0.14, 0.9, 0.8]
}

style "laptop" {
  icon = file("icons/laptop.svg")
}

style "router" {
  icon = file("icons/router.svg")
}

style "switch" {
  icon = file("icons/switch.svg")
}

style "access-point" {
  icon = file("icons/access-point.svg")
}

style "server" {
  icon = file("icons/server.svg")
}

style "nas" {
  icon = file("icons/nas.svg")
}

style "rack" {
  shape = "rack"
}

edge_style "ethernet" {
  stroke       = "#1F3A5F"
  stroke_width = 1.5
}

edge_style "wifi" {
  stroke      = "#1A73E8"
  stroke_dash = [4, 3]
}
