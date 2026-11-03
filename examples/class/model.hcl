# A UML class diagram: classes with field and method compartments, inheritance, realization,
# composition, aggregation and associations with multiplicities at their ends.

node "Customer" {
  kind = "class"
  compartment "fields" {
    row "name" {
      type = "String"
    }
    row "email" {
      type = "String"
    }
  }
  compartment "methods" {
    row "placeOrder()" {
      type = "Order"
    }
  }
}

node "Order" {
  kind = "class"
  compartment "fields" {
    row "placedAt" {
      type = "Instant"
    }
    row "status" {
      type = "Status"
    }
  }
  compartment "methods" {
    row "total()" {
      type = "Money"
    }
    row "cancel()" {}
  }
}

node "OrderLine" {
  kind = "class"
  compartment "fields" {
    row "quantity" {
      type = "int"
    }
  }
  compartment "methods" {
    row "subtotal()" {
      type = "Money"
    }
  }
}

node "Product" {
  kind = "class"
  compartment "fields" {
    row "sku" {
      type = "String"
    }
    row "price" {
      type = "Money"
    }
  }
  compartment "methods" {}
}

node "Category" {
  kind = "class"
  compartment "fields" {
    row "name" {
      type = "String"
    }
  }
  compartment "methods" {}
}

node "Address" {
  kind = "class"
  compartment "fields" {
    row "street" {
      type = "String"
    }
    row "city" {
      type = "String"
    }
  }
  compartment "methods" {}
}

node "Payable" {
  kind  = "class"
  label = "«interface»\nPayable"
  compartment "methods" {
    row "pay()" {
      type = "Receipt"
    }
  }
}

node "Payment" {
  kind = "class"
  compartment "fields" {
    row "amount" {
      type = "Money"
    }
  }
  compartment "methods" {
    row "pay()" {
      type = "Receipt"
    }
  }
}

node "CardPayment" {
  kind = "class"
  compartment "fields" {
    row "cardNumber" {
      type = "String"
    }
  }
  compartment "methods" {}
}

node "BankTransfer" {
  kind = "class"
  compartment "fields" {
    row "iban" {
      type = "String"
    }
  }
  compartment "methods" {}
}

edge "places" {
  from   = node.Customer
  to     = node.Order
  kind   = "association"
  labels = { start = "1", mid = "places", end = "0..*" }
}

edge "lines" {
  from   = node.Order
  to     = node.OrderLine
  kind   = "composition"
  labels = { start = "1", end = "1..*" }
}

edge "line-product" {
  from   = node.OrderLine
  to     = node.Product
  kind   = "association"
  labels = { end = "1" }
}

edge "category-products" {
  from   = node.Category
  to     = node.Product
  kind   = "aggregation"
  labels = { start = "0..1", end = "*" }
}

# A self-association: categories nest.
edge "subcategories" {
  from   = node.Category
  to     = node.Category
  kind   = "association"
  labels = { start = "0..1", end = "*" }
}

# Two associations between the same pair of classes.
edge "ship-to" {
  from   = node.Order
  to     = node.Address
  kind   = "association"
  labels = { mid = "shipTo", end = "1" }
}

edge "bill-to" {
  from   = node.Order
  to     = node.Address
  kind   = "association"
  labels = { mid = "billTo", end = "0..1" }
}

edge "order-payment" {
  from   = node.Order
  to     = node.Payment
  kind   = "association"
  labels = { start = "1", mid = "paid by", end = "0..1" }
}

edge "payment-payable" {
  from = node.Payment
  to   = node.Payable
  kind = "realizes"
}

edge "card-payment" {
  from = node.CardPayment
  to   = node.Payment
  kind = "inherits"
}

edge "bank-payment" {
  from = node.BankTransfer
  to   = node.Payment
  kind = "inherits"
}
