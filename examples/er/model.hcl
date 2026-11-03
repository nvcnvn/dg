# An entity-relationship diagram of a shop: tables with typed, keyed rows, and foreign keys
# drawn from row to row with crow's-foot cardinality at both ends.

node "customers" {
  kind = "table"
  row "id" {
    type = "uuid"
    key  = "PK"
  }
  row "email" {
    type = "text"
  }
  row "name" {
    type = "text"
  }
}

node "addresses" {
  kind = "table"
  row "id" {
    type = "uuid"
    key  = "PK"
  }
  row "customer_id" {
    type = "uuid"
    key  = "FK"
  }
  row "city" {
    type = "text"
  }
}

node "orders" {
  kind = "table"
  row "id" {
    type = "uuid"
    key  = "PK"
  }
  row "customer_id" {
    type = "uuid"
    key  = "FK"
  }
  row "ship_to" {
    type = "uuid"
    key  = "FK"
  }
  row "bill_to" {
    type = "uuid"
    key  = "FK"
  }
  row "placed_at" {
    type = "timestamp"
  }
}

node "order_items" {
  kind = "table"
  row "order_id" {
    type = "uuid"
    key  = "PK FK"
  }
  row "product_id" {
    type = "uuid"
    key  = "PK FK"
  }
  row "quantity" {
    type = "int"
  }
}

node "products" {
  kind = "table"
  row "id" {
    type = "uuid"
    key  = "PK"
  }
  row "category_id" {
    type = "uuid"
    key  = "FK"
  }
  row "name" {
    type = "text"
  }
  row "price" {
    type = "numeric(10,2)"
  }
}

node "categories" {
  kind = "table"
  row "id" {
    type = "uuid"
    key  = "PK"
  }
  row "parent_id" {
    type = "uuid"
    key  = "FK"
  }
  row "name" {
    type = "text"
  }
}

edge "address-customer" {
  from = node.addresses.row.customer_id
  to   = node.customers.row.id
  tags = ["start-zero-many", "end-one"]
}

edge "order-customer" {
  from = node.orders.row.customer_id
  to   = node.customers.row.id
  tags = ["start-zero-many", "end-one"]
}

# Two foreign keys between the same pair of tables.
edge "order-ship-to" {
  from  = node.orders.row.ship_to
  to    = node.addresses.row.id
  label = "ships to"
  tags  = ["start-zero-many", "end-one"]
}

edge "order-bill-to" {
  from  = node.orders.row.bill_to
  to    = node.addresses.row.id
  label = "bills to"
  tags  = ["start-zero-many", "end-zero-one"]
}

edge "item-order" {
  from = node.order_items.row.order_id
  to   = node.orders.row.id
  tags = ["start-one-many", "end-one"]
}

edge "item-product" {
  from = node.order_items.row.product_id
  to   = node.products.row.id
  tags = ["start-many", "end-one"]
}

edge "product-category" {
  from = node.products.row.category_id
  to   = node.categories.row.id
  tags = ["start-zero-many", "end-one"]
}

# A self-reference: a category's parent is another category.
edge "category-parent" {
  from = node.categories.row.parent_id
  to   = node.categories.row.id
  tags = ["start-zero-many", "end-zero-one"]
}
