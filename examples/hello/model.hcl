node "user" {
  label = "User"
}

node "backend" {
  kind  = "group"
  label = "Backend"
}

node "api" {
  kind   = "service"
  label  = "API"
  parent = node.backend
}

node "worker" {
  kind   = "service"
  label  = "Worker"
  parent = node.backend
}

node "orders-db" {
  kind   = "database"
  label  = "Orders DB"
  parent = node.backend
}

edge "user-api" {
  from  = node.user
  to    = node.api
  label = "HTTPS"
}

edge "api-worker" {
  from  = node.api
  to    = node.worker
  kind  = "async"
  label = "jobs"
}

edge "api-db" {
  from = node.api
  to   = node["orders-db"]
}

edge "worker-db" {
  from = node.worker
  to   = node["orders-db"]
}
