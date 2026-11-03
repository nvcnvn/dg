# Every built-in shape, custom shapes, icons and the node and edge style properties.

node "rect" {
  tags = ["rect"]
}

node "rounded" {
  kind = "service"
}

node "stadium" {
  kind = "terminal"
}

node "ellipse" {
  tags = ["ellipse"]
}

node "diamond" {
  kind = "decision"
}

node "cylinder" {
  kind = "database"
}

node "hexagon" {
  tags = ["hexagon"]
}

node "parallelogram" {
  kind = "queue"
}

node "document" {
  kind  = "note"
  label = "document: a note whose label is long enough to wrap at max_w"
}

node "person" {
  kind = "person"
}

node "cloud" {
  tags = ["cloud"]
}

node "text" {
  label = "text: no outline"
  tags  = ["text"]
}

node "image" {
  kind = "picture"
}

node "point" {
  kind = "initial"
}

node "bullseye" {
  kind = "final"
}

node "table" {
  kind = "table"
}

node "external" {
  kind = "external"
}

node "locked" {
  label = "icon"
  kind  = "service"
  tags  = ["locked"]
}

node "bold" {
  label = "font_weight 700"
  tags  = ["bold"]
}

node "faded" {
  label = "opacity 0.5"
  tags  = ["faded"]
}

node "rack" {
  kind  = "rack"
  label = "nine-slice rack"
}

node "chip" {
  kind  = "chip"
  label = "uniform chip"
}

node "lab" {
  kind  = "group"
  label = "Lab"
}

node "bench" {
  kind   = "service"
  label  = "Bench"
  parent = node.lab
  tags   = ["on-top"]
  port "probe" {
    side = "right"
  }
}

edge "rack-chip" {
  from  = node.rack.port.down
  to    = node.chip.port.in
  label = "anchors"
}

edge "chip-bench" {
  from  = node.chip.port.out
  to    = node.bench.port.probe
  tags  = ["both"]
  label = "ports, arrows at both ends"
}

edge "lab-bench" {
  from  = node.lab
  to    = node.bench
  label = "to a child"
}

edge "point-rounded" {
  from = node.point
  to   = node.rounded
}

edge "rounded-diamond" {
  from = node.rounded
  to   = node.diamond
  tags = ["polyline"]
}

edge "diamond-bullseye" {
  from = node.diamond
  to   = node.bullseye
  tags = ["straight"]
}

edge "diamond-cylinder" {
  from  = node.diamond
  to    = node.cylinder
  kind  = "async"
  label = "async"
}
