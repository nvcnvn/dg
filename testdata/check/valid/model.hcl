node "tor-1" {
  kind  = "tor"
  label = file("label.txt")
  ports = port_range("p", 1, 4)
  props = { rack = 3, speed = "400G" }
  port "uplink" {
    side = "top"
  }
}

node "db" {
  kind = "table"
  compartment "keys" {
    row "id" {
      type = "uuid"
      key  = "pk"
    }
  }
  compartment "fields" {
    row "save()" {}
  }
}

edge "link" {
  from   = node["tor-1"].port.p1
  to     = node.db.row["save()"]
  kind   = "rdma"
  tags   = ["hot"]
  labels = { start = "1", mid = "link", end = "n" }
}
