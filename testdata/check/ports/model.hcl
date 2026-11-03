node "tor" {
  kind  = "switch"
  ports = port_range("p", 1, 4)
  port "p2" {}
  port "mgmt" {
    side = "right"
  }
  port "uplink" {
    side = "middle"
  }
}

node "big" {
  ports = port_range("p", 1, 20000)
}

node "backwards" {
  ports = port_range("p", 5, 1)
}

node "fraction" {
  ports = port_range("p", 1.5, 3)
}

node "listed" {
  ports = ["a", "a", ""]
}
