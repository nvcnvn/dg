style "vpc" {
  icon = file("icons/vpc.svg")
}

style "lambda" {
  shape = "hex"
}

style "edge-lambda" {
  shape = "rounded"
}

style "tag.legacy" {
  fill = "#EEEEEE"
}

edge_style "peering" {
  stroke = "#FF9900"
}

overlay_style "zone" {
  stroke = "#FF9900"
}

shape "hex" {
  svg = file("icons/hex.svg")
}
