# The Internet Banking System of the C4 model's example, at three levels: the system, its
# containers, and the components of its API application. Relationships are written between the
# most detailed elements; a view that collapses a system or a container lifts them to it.

node "customer" {
  kind  = "c4-person"
  label = "Personal Banking Customer\n[Person]\n\nA customer of the bank, with personal bank accounts."
}

node "banking" {
  kind  = "system"
  label = "Internet Banking System\n[Software System]\n\nAllows customers to view information about their bank accounts, and make payments."
}

node "mainframe" {
  kind  = "system"
  tags  = ["existing"]
  label = "Mainframe Banking System\n[Software System]\n\nStores all of the core banking information about customers, accounts, transactions, etc."
}

node "email" {
  kind  = "external-system"
  label = "E-mail System\n[Software System]\n\nThe internal Microsoft Exchange e-mail system."
}

# Containers of the Internet Banking System.

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
  kind   = "app-host"
  parent = node.banking
  label  = "API Application\n[Container: Java and Spring MVC]\n\nProvides Internet banking functionality via a JSON/HTTPS API."
}

node "database" {
  kind   = "store"
  parent = node.banking
  label  = "Database\n[Container: Oracle Database Schema]\n\nStores user registration information, hashed authentication credentials, access logs, etc."
}

# Containers of the mainframe, which no view opens.

node "core" {
  kind   = "app"
  parent = node.mainframe
  label  = "Core Banking\n[Container: COBOL]"
}

node "ledger" {
  kind   = "store"
  parent = node.mainframe
  label  = "Ledger\n[Container: DB2]"
}

# Components of the API application.

node "signin" {
  kind   = "component"
  parent = node.api
  label  = "Sign In Controller\n[Component: Spring MVC Rest Controller]\n\nAllows users to sign in to the Internet Banking System."
}

node "accounts" {
  kind   = "component"
  parent = node.api
  label  = "Accounts Summary Controller\n[Component: Spring MVC Rest Controller]\n\nProvides customers with a summary of their bank accounts."
}

node "reset" {
  kind   = "component"
  parent = node.api
  label  = "Reset Password Controller\n[Component: Spring MVC Rest Controller]\n\nAllows users to reset their passwords with a single use URL."
}

node "security" {
  kind   = "component"
  parent = node.api
  label  = "Security Component\n[Component: Spring Bean]\n\nProvides functionality related to signing in, changing passwords, etc."
}

node "facade" {
  kind   = "component"
  parent = node.api
  label  = "Mainframe Banking System Facade\n[Component: Spring Bean]\n\nA facade onto the mainframe banking system."
}

node "mailer" {
  kind   = "component"
  parent = node.api
  label  = "E-mail Component\n[Component: Spring Bean]\n\nSends e-mails to users."
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
  label = "Views account balances,\nand makes payments using"
}

edge "customer-mobile" {
  from  = node.customer
  to    = node["mobile-app"]
  kind  = "uses"
  label = "Views account balances,\nand makes payments using"
}

edge "web-spa" {
  from  = node["web-app"]
  to    = node.spa
  kind  = "uses"
  label = "Delivers to the\ncustomer's web browser"
}

edge "spa-signin" {
  from  = node.spa
  to    = node.signin
  kind  = "uses"
  label = "Makes API calls to\n[JSON/HTTPS]"
}

edge "spa-accounts" {
  from  = node.spa
  to    = node.accounts
  kind  = "uses"
  label = "Makes API calls to\n[JSON/HTTPS]"
}

edge "spa-reset" {
  from  = node.spa
  to    = node.reset
  kind  = "uses"
  label = "Makes API calls to\n[JSON/HTTPS]"
}

edge "mobile-signin" {
  from  = node["mobile-app"]
  to    = node.signin
  kind  = "uses"
  label = "Makes API calls to\n[JSON/HTTPS]"
}

edge "mobile-accounts" {
  from  = node["mobile-app"]
  to    = node.accounts
  kind  = "uses"
  label = "Makes API calls to\n[JSON/HTTPS]"
}

edge "signin-security" {
  from  = node.signin
  to    = node.security
  kind  = "uses"
  label = "Uses"
}

edge "reset-security" {
  from  = node.reset
  to    = node.security
  kind  = "uses"
  label = "Uses"
}

edge "reset-mailer" {
  from  = node.reset
  to    = node.mailer
  kind  = "uses"
  label = "Uses"
}

edge "accounts-facade" {
  from  = node.accounts
  to    = node.facade
  kind  = "uses"
  label = "Uses"
}

edge "security-database" {
  from  = node.security
  to    = node.database
  kind  = "uses"
  label = "Reads from and writes to\n[SQL/TCP]"
}

edge "facade-core" {
  from  = node.facade
  to    = node.core
  kind  = "uses"
  label = "Makes API calls to\n[XML/HTTPS]"
}

edge "core-ledger" {
  from = node.core
  to   = node.ledger
  kind = "uses"
}

edge "mailer-email" {
  from  = node.mailer
  to    = node.email
  kind  = "uses"
  label = "Sends e-mail using"
}

edge "email-customer" {
  from  = node.email
  to    = node.customer
  kind  = "uses"
  label = "Sends e-mails to"
}
