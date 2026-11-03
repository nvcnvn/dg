# The container diagram of the C4 model's Internet Banking System example.

node "customer" {
  kind  = "c4-person"
  label = "Personal Banking Customer\n[Person]\n\nA customer of the bank, with personal bank accounts."
}

node "banking" {
  kind  = "boundary"
  label = "Internet Banking System\n[Software System]"
}

node "web-app" {
  kind   = "app"
  parent = node.banking
  label  = "Web Application\n[Container: Java and Spring MVC]\n\nDelivers the static content and the Internet banking single page application."
}

node "spa" {
  kind   = "app"
  parent = node.banking
  label  = "Single-Page Application\n[Container: JavaScript and Angular]\n\nProvides all of the Internet banking functionality to customers via their web browser."
}

node "mobile-app" {
  kind   = "app"
  parent = node.banking
  label  = "Mobile App\n[Container: Xamarin]\n\nProvides a limited subset of the Internet banking functionality to customers via their mobile device."
}

node "api" {
  kind   = "app"
  parent = node.banking
  label  = "API Application\n[Container: Java and Spring MVC]\n\nProvides Internet banking functionality via a JSON/HTTPS API."
}

node "database" {
  kind   = "store"
  parent = node.banking
  label  = "Database\n[Container: Oracle Database Schema]\n\nStores user registration information, hashed credentials, access logs, etc."
}

node "email" {
  kind  = "external-system"
  label = "E-mail System\n[Software System]\n\nThe internal Microsoft Exchange e-mail system."
}

node "mainframe" {
  kind  = "external-system"
  label = "Mainframe Banking System\n[Software System]\n\nStores all of the core banking information about customers, accounts, transactions, etc."
}

edge "customer-web" {
  from  = node.customer
  to    = node["web-app"]
  kind  = "uses"
  label = "Visits bigbank.com/ib\n[HTTPS]"
}

edge "customer-spa" {
  from  = node.customer
  to    = node.spa
  kind  = "uses"
  label = "Views balances,\nmakes payments"
}

edge "customer-mobile" {
  from  = node.customer
  to    = node["mobile-app"]
  kind  = "uses"
  label = "Views balances,\nmakes payments"
}

edge "web-spa" {
  from  = node["web-app"]
  to    = node.spa
  kind  = "uses"
  label = "Delivers to the\ncustomer's browser"
}

edge "spa-api" {
  from  = node.spa
  to    = node.api
  kind  = "uses"
  label = "API calls\n[JSON/HTTPS]"
}

edge "mobile-api" {
  from  = node["mobile-app"]
  to    = node.api
  kind  = "uses"
  label = "API calls\n[JSON/HTTPS]"
}

edge "api-database" {
  from  = node.api
  to    = node.database
  kind  = "uses"
  label = "Reads from and writes to\n[SQL/TCP]"
}

edge "api-mainframe" {
  from  = node.api
  to    = node.mainframe
  kind  = "uses"
  label = "Uses\n[XML/HTTPS]"
}

edge "api-email" {
  from  = node.api
  to    = node.email
  kind  = "uses"
  label = "Sends e-mail using\n[SMTP]"
}

edge "email-customer" {
  from  = node.email
  to    = node.customer
  kind  = "uses"
  label = "Sends e-mails to"
}

# The steps of signing in, shown by the sign-in view.

edge "sign-in-1" {
  from  = node.customer
  to    = node.spa
  kind  = "interaction"
  label = "1. Submits credentials to"
}

edge "sign-in-2" {
  from  = node.spa
  to    = node.api
  kind  = "interaction"
  label = "2. Sends credentials to\n[JSON/HTTPS]"
}

edge "sign-in-3" {
  from  = node.api
  to    = node.database
  kind  = "interaction"
  label = "3. Validates credentials\nagainst\n[SQL/TCP]"
}

edge "sign-in-4" {
  from  = node.api
  to    = node.spa
  kind  = "interaction"
  label = "4. Returns a session token"
}
