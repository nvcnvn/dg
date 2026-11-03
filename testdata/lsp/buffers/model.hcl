node "api" {
  kind = "service"
}

node "db" {
  kind = "database"
}

edge "api-db" {
  from = node.api
  to   = node.db
}
