kind "a" {
  extends = "b"
}

kind "b" {
  extends = "a"
}

kind "c" {
  extends = "zzz"
}

kind "x" {}

kind "x" {}

edge_kind "y" {}

edge_kind "y" {}

kind "base" {
  ports             = ["nic0", "nic1", "nic0"]
  default_port_side = "middle"
  property "gpu_count" {
    type    = number
    default = 8
  }
  property "gpu_count" {
    type = number
  }
  property "zone" {
    type    = string
    default = 3
  }
  property "fast" {
    type = integer
  }
  property "flag" {}
}

kind "sub" {
  extends = "base"
  ports   = ["nic1", "nic2"]
  property "gpu_count" {
    type = number
  }
  property "rack" {
    type    = bool
    default = true
  }
}
