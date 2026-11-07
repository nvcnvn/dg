kind "laptop" {}

kind "router" {
  category = "Network"
}

kind "switch" {
  category = "Network"
}

kind "access-point" {
  category = "Network"
}

kind "server" {
  category = "Compute"
}

kind "rack" {
  extends  = "group"
  category = "Compute"
}

kind "nas" {
  category = "Storage"
}

edge_kind "ethernet" {}

edge_kind "wifi" {}
