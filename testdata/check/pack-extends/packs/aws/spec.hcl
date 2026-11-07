kind "ok" {
  extends = "group"
}

kind "mine" {
  extends = "ok"
}

kind "other" {
  extends = "gcp-vm"
}

kind "nowhere" {
  extends = "vm"
}
