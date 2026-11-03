# A state whose concurrent regions run side by side, stacked inside it.

kind "regions" {
  extends = "state"
}

kind "region" {
  container = true
}
