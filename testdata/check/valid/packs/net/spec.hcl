kind "switch" {
  ports             = ["mgmt"]
  default_port_side = "bottom"
  property "speed" {
    type    = string
    default = "100G"
  }
}

kind "tor" {
  extends = "switch"
  property "rack" {
    type = number
  }
}

kind "table" {
  rows = true
}

edge_kind "rdma" {}
