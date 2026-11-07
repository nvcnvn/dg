node "internet" {
  kind  = "external"
  label = "Internet"
}

node "gateway" {
  kind  = "homelab-router"
  label = "Gateway"
}

node "core" {
  kind  = "homelab-switch"
  label = "Core switch"
}

node "hall" {
  kind  = "homelab-access-point"
  label = "Hall Wi-Fi"
}

node "laptop" {
  kind  = "homelab-laptop"
  label = "Laptop"
}

node "cabinet" {
  kind  = "homelab-rack"
  label = "Cabinet"
}

node "hypervisor" {
  kind   = "homelab-server"
  parent = node.cabinet
  label  = "Hypervisor"
}

node "backups" {
  kind   = "homelab-nas"
  parent = node.cabinet
  label  = "Backups"
}

edge "uplink" {
  from = node.internet
  to   = node.gateway
}

edge "gateway-core" {
  from = node.gateway
  to   = node.core
  kind = "homelab-ethernet"
}

edge "core-hall" {
  from = node.core
  to   = node.hall
  kind = "homelab-ethernet"
}

edge "core-hypervisor" {
  from = node.core
  to   = node.hypervisor
  kind = "homelab-ethernet"
}

edge "hypervisor-backups" {
  from = node.hypervisor
  to   = node.backups
  kind = "homelab-ethernet"
}

edge "laptop-hall" {
  from = node.laptop
  to   = node.hall
  kind = "homelab-wifi"
}
