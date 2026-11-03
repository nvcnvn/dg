# Every lifting and merging case of spec 8.3, one view each (views.hcl).

node "client" {
  kind = "service"
  port "out" {
    side = "right"
  }
}

node "ext" {
  kind = "external"
}

node "sys" {
  kind = "group"
}

node "web" {
  kind   = "service"
  parent = node.sys
}

node "queue" {
  kind   = "queue"
  parent = node.sys
}

node "api" {
  kind   = "group"
  parent = node.sys
}

node "handler" {
  parent = node.api
  port "in" {
    side = "left"
  }
}

node "store" {
  kind   = "database"
  parent = node.api
}

edge "client-web" {
  from  = node.client.port.out
  to    = node.web
  kind  = "async"
  tags  = ["hot"]
  label = "HTTPS"
}

edge "client-queue" {
  from  = node.client.port.out
  to    = node.queue
  kind  = "async"
  label = "enqueue"
}

edge "client-handler" {
  from   = node.client
  to     = node.handler.port.in
  tags   = ["hot"]
  labels = { start = "1", mid = "calls", end = "n" }
}

edge "client-sys" {
  from  = node.client
  to    = node.sys
  label = "direct"
}

edge "handler-store" {
  from = node.handler
  to   = node.store
}

edge "web-handler" {
  from = node.web
  to   = node.handler
}

edge "handler-ext" {
  from = node.handler
  to   = node.ext
  kind = "async"
}

edge "store-ext" {
  from = node.store
  to   = node.ext
  kind = "async"
}

# A collapsed container whose quoted ID holds the characters a layout key encodes.

node "rack:a%b#c" {
  kind = "group"
}

node "disk" {
  kind   = "storage"
  parent = node["rack:a%b#c"]
}

edge "client-disk" {
  from = node.client
  to   = node.disk
}
