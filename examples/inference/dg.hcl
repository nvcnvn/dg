project "inference" {
  packs     = ["generic", "packs/inference"]
  models    = ["model.hcl", "generated/*.hcl"]
  read_only = ["generated/*.hcl"]
}
