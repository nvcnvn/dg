# An order fulfilment process across the teams that run it: a lane set holds one lane per team,
# stacked in the order the layout files give, and each lane holds its steps freely placed.

node "fulfilment" {
  kind  = "lane-set"
  label = "Order fulfilment"
}

node "customer" {
  kind   = "lane"
  label  = "Customer"
  parent = node.fulfilment
}

node "sales" {
  kind   = "lane"
  label  = "Sales"
  parent = node.fulfilment
}

node "warehouse" {
  kind   = "lane"
  label  = "Warehouse"
  parent = node.fulfilment
}

node "billing" {
  kind   = "lane"
  label  = "Billing"
  parent = node.fulfilment
}

node "order" {
  kind   = "terminal"
  label  = "Place order"
  parent = node.customer
}

node "receive" {
  kind   = "terminal"
  label  = "Receive goods"
  parent = node.customer
}

node "check" {
  kind   = "node"
  label  = "Check order"
  parent = node.sales
}

node "in-stock" {
  kind   = "decision"
  label  = "In stock?"
  parent = node.sales
}

node "backorder" {
  kind   = "node"
  label  = "Backorder"
  parent = node.sales
}

node "pick" {
  kind   = "node"
  label  = "Pick items"
  parent = node.warehouse
}

node "ship" {
  kind   = "node"
  label  = "Ship"
  parent = node.warehouse
}

node "invoice" {
  kind   = "node"
  label  = "Send invoice"
  parent = node.billing
}

node "payment" {
  kind   = "node"
  label  = "Collect payment"
  parent = node.billing
}

edge "order-check" {
  from = node.order
  to   = node.check
}

edge "check-in-stock" {
  from = node.check
  to   = node.in-stock
}

edge "in-stock-pick" {
  from  = node.in-stock
  to    = node.pick
  label = "yes"
}

edge "in-stock-backorder" {
  from  = node.in-stock
  to    = node.backorder
  label = "no"
}

edge "backorder-pick" {
  from  = node.backorder
  to    = node.pick
  kind  = "async"
  label = "restocked"
}

edge "pick-ship" {
  from = node.pick
  to   = node.ship
}

edge "ship-receive" {
  from = node.ship
  to   = node.receive
}

edge "ship-invoice" {
  from = node.ship
  to   = node.invoice
}

edge "invoice-payment" {
  from = node.invoice
  to   = node.payment
}
