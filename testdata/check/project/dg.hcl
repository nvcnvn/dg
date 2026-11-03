project "demo" {
  packs      = ["generic", "packs/missing", "../outside", "packs/local"]
  models     = ["model.hcl", "generated/*.hcl", "[bad"]
  read_only  = ["generated/*.hcl", "[bad"]
  grid       = 0
  font       = "fonts/missing.ttf"
  layout_dir = "/abs"
}

project "second" {}
