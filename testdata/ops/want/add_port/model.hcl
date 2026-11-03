# The model the operation tests edit. Its formatting is deliberately not canonical: every
# operation must leave what it does not change byte for byte.

node   "gateway" {
  kind  =   "service" # the entry point
  label = "Gateway"   /* shown on top */
  tags  = [ "edge" ]
}

node "pool" {
    kind = "group"
    label = <<-EOT
      GPU Pool
      EOT
}

node "worker-1" {
  kind   = "worker"
  parent = node.pool
  props  = { gpus = 8 }
  port "nic0" {
    side = "right"
  }
  port "nic1" {}
}

node "worker-2" {
  kind   = "worker"
  parent = node.pool
  port "nic0" {
    side = "left"
  }
}

node "cache" { kind = "database" }

# Storage, kept apart.
node "rack" {
  kind = "group"
}

node "disk" {
  kind   = "storage"
  parent =   node.rack   # in the rack
}

node "orders" {
  kind = "table"
  row "id" {
    type = "uuid"
    key  = "pk"
  }
}

node "kube-system/dns" {
  kind = "service"
}

edge "gw-w1" {
  from  = node.gateway
  to    = node.worker-1.port.nic0
  label = "rpc"
}

edge "w1-w2" {
  from = node["worker-1"]
  to = node.worker-2
  kind = "async"   # queued
}

edge "dns-gw" {
  from =  node["kube-system/dns"] // a line comment
  to   = node.gateway
}
