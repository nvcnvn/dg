# The states of an order. Processing runs two regions at once, payment and shipping, stacked
# inside it; each region holds its states freely placed.

node "start" {
  kind = "initial"
}

node "created" {
  kind  = "simple-state"
  label = "Created"
}

node "processing" {
  kind  = "regions"
  label = "Processing"
}

node "payment" {
  kind   = "region"
  label  = "Payment"
  parent = node.processing
}

node "pay-start" {
  kind   = "initial"
  parent = node.payment
}

node "authorizing" {
  kind   = "simple-state"
  label  = "Authorizing"
  parent = node.payment
}

node "paid" {
  kind   = "simple-state"
  label  = "Paid"
  parent = node.payment
}

node "pay-end" {
  kind   = "final"
  parent = node.payment
}

node "shipping" {
  kind   = "region"
  label  = "Shipping"
  parent = node.processing
}

node "ship-start" {
  kind   = "initial"
  parent = node.shipping
}

node "picking" {
  kind   = "simple-state"
  label  = "Picking"
  parent = node.shipping
}

node "packed" {
  kind   = "simple-state"
  label  = "Packed"
  parent = node.shipping
}

node "ship-end" {
  kind   = "final"
  parent = node.shipping
}

node "delivered" {
  kind  = "simple-state"
  label = "Delivered"
}

node "cancelled" {
  kind  = "simple-state"
  label = "Cancelled"
}

node "end" {
  kind = "final"
}

edge "start-created" {
  from = node.start
  to   = node.created
}

edge "created-processing" {
  from  = node.created
  to    = node.processing
  label = "submit"
}

edge "created-cancelled" {
  from  = node.created
  to    = node.cancelled
  label = "cancel"
}

edge "created-edit" {
  from  = node.created
  to    = node.created
  label = "edit"
}

edge "pay-start-authorizing" {
  from = node.pay-start
  to   = node.authorizing
}

edge "authorizing-paid" {
  from  = node.authorizing
  to    = node.paid
  label = "authorized"
}

edge "paid-pay-end" {
  from = node.paid
  to   = node.pay-end
}

edge "ship-start-picking" {
  from = node.ship-start
  to   = node.picking
}

edge "picking-packed" {
  from  = node.picking
  to    = node.packed
  label = "packed"
}

edge "packed-ship-end" {
  from = node.packed
  to   = node.ship-end
}

edge "processing-delivered" {
  from  = node.processing
  to    = node.delivered
  label = "delivered"
}

edge "processing-cancelled" {
  from  = node.processing
  to    = node.cancelled
  label = "declined"
}

edge "delivered-end" {
  from = node.delivered
  to   = node.end
}

edge "cancelled-end" {
  from = node.cancelled
  to   = node.end
}
