node "plain" {
  row "id" {}
}

node "t" {
  kind = "table"
  row "id" {
    type = "uuid"
    key  = "pk"
  }
  row "id" {}
  compartment "c" {
    row "x" {}
  }
}

node "class" {
  kind = "table"
  compartment "fields" {
    row "name" {}
  }
  compartment "fields" {}
  compartment "methods" {
    row "name" {}
    row "save()" {
      size = 3
    }
  }
}
