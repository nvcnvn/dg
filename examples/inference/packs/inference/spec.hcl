kind "gpu-node" {
  ports             = ["nic0", "nic1"]
  default_port_side = "auto"
  property "gpu_count" {
    type    = number
    default = 8
  }
}

kind "switch" {
  property "speed" {
    type    = string
    default = "100G"
  }
}

kind "storage" {}

edge_kind "rdma" {}
