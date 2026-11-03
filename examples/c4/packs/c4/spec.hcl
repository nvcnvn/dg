# C4 model vocabulary (https://c4model.com) for diagrams at three levels: software systems hold
# containers, and a container may hold components.

kind "c4-person" {
  extends = "person"
}

kind "system" {
  container = true
}

kind "external-system" {}

kind "app" {}

kind "app-host" {
  extends   = "app"
  container = true
}

kind "store" {}

kind "component" {}

edge_kind "uses" {}
