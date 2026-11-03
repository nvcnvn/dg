# Hand-written model; comments survive formatting.
node "gateway" {
  kind  = "service"
  label = "Gateway" # trailing comment
}

node "sys:ctl" {
  kind  = "switch"
  ports = port_range("p", 1, 2)
}

edge "gw-ctl" {
  from = node.gateway
  to   = node["sys:ctl"].port.p1
}
