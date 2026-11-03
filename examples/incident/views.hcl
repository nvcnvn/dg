view "main" {
  title = "Checkout platform"
}

# The incident board: what the payment provider outage reaches, with the data tier collapsed.
view "incident" {
  title    = "Incident: payment provider timeouts"
  collapse = [node.data]

  overlay "blast-radius" {
    members = [tag.degraded]
    style   = "hull-red"
    label   = "Blast radius"
  }

  style "tag.degraded" {
    fill   = "#FCE8E6"
    stroke = "#D93025"
  }

  style "node.payments" {
    stroke_width = 3
  }

  edge_style "edge.payments-psp" {
    stroke  = "#D93025"
    animate = "flow"
  }
}
