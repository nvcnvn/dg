project "valid" {
  packs     = ["generic", "packs/net"]
  models    = ["*.hcl", "generated/*.hcl"]
  read_only = ["generated/*.hcl"]
  grid      = 20
}
