# main draws the lane set; lanes, whose of is the lane set, draws the lanes alone, stacked by
# its policy.
view "main" {
  title = "Order fulfilment"
}

view "lanes" {
  title = "Order fulfilment, lanes only"
  of    = node.fulfilment
}
