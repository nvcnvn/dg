# A checkout platform. The incident view outlines what a payment provider outage reaches.

node "edge" {
  kind  = "group"
  label = "Edge"
}

node "lb" {
  kind   = "service"
  parent = node.edge
  label  = "Load balancer"
}

node "gateway" {
  kind   = "service"
  parent = node.edge
  label  = "API gateway"
}

node "core" {
  kind  = "group"
  label = "Core services"
}

node "orders" {
  kind   = "service"
  parent = node.core
  label  = "Orders"
}

node "payments" {
  kind   = "service"
  parent = node.core
  label  = "Payments"
  tags   = ["degraded"]
}

node "inventory" {
  kind   = "service"
  parent = node.core
  label  = "Inventory"
}

node "events" {
  kind  = "queue"
  label = "Order events"
}

node "data" {
  kind  = "group"
  label = "Data"
}

node "orders-db" {
  kind   = "database"
  parent = node.data
  label  = "Orders DB"
}

node "payments-db" {
  kind   = "database"
  parent = node.data
  label  = "Payments DB"
  tags   = ["degraded"]
}

node "cache" {
  kind   = "database"
  parent = node.data
  label  = "Stock cache"
}

node "psp" {
  kind  = "external"
  label = "Payment provider"
  tags  = ["degraded"]
}

edge "lb-gateway" {
  from = node.lb
  to   = node.gateway
}

edge "gateway-orders" {
  from  = node.gateway
  to    = node.orders
  label = "checkout"
}

edge "orders-payments" {
  from  = node.orders
  to    = node.payments
  label = "charge"
}

edge "orders-inventory" {
  from  = node.orders
  to    = node.inventory
  label = "reserve"
}

edge "orders-db" {
  from = node.orders
  to   = node["orders-db"]
}

edge "payments-db" {
  from = node.payments
  to   = node["payments-db"]
}

edge "inventory-cache" {
  from = node.inventory
  to   = node.cache
}

edge "payments-psp" {
  from  = node.payments
  to    = node.psp
  label = "authorize"
}

edge "orders-events" {
  from = node.orders
  to   = node.events
  kind = "async"
}

edge "payments-events" {
  from = node.payments
  to   = node.events
  kind = "async"
}
