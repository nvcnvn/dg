# Level 1: the system context. Every software system is collapsed, so the relationships of
# their containers and components are lifted to them.
view "main" {
  title    = "Internet Banking System: system context"
  collapse = [kind.system]
}

# Level 2: the containers of the Internet Banking System, opened from its drill-down badge. The
# API application, whose components level 3 shows, stays collapsed.
view "containers" {
  title    = "Internet Banking System: containers"
  of       = node.banking
  include  = [descendants.banking, node.banking, node.customer, node.mainframe, node.email]
  collapse = [node.api, node.mainframe]

  style "node.banking" {
    shape       = "rect"
    fill        = "none"
    stroke      = "#444444"
    stroke_dash = [8, 4]
    text_color  = "#444444"
    padding     = 20
  }
}

# Level 3: the components of the API application.
view "components" {
  title    = "API Application: components"
  of       = node.api
  include  = [descendants.api, node.api, node.spa, node["mobile-app"], node.database, node.mainframe, node.email]
  collapse = [node.mainframe]

  style "node.api" {
    shape       = "rect"
    fill        = "none"
    stroke      = "#444444"
    stroke_dash = [8, 4]
    text_color  = "#444444"
    padding     = 20
  }
}
