node "a" {
  kind  = "service"
  color = "red"

  port "p" {
    position = "top"
  }

  row "r" {
    width = 3
  }

  compartment "c" {
    title = "c"
  }

  widget "w" {
  }
}

edge "e" {
  from  = node.a
  to    = node.a
  style = "dashed"
}
