# C4 model vocabulary (https://c4model.com): people, software systems, containers and the
# boundary of the system whose containers a container diagram shows.

kind "c4-person" {
  extends = "person"
}

kind "system" {}

kind "external-system" {
  extends = "system"
}

kind "app" {}

kind "store" {}

kind "boundary" {
  container = true
}

edge_kind "uses" {}

# A numbered step of a dynamic diagram: one interaction of a scenario, drawn animated.
edge_kind "interaction" {}
