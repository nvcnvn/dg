project "fmt" {
  packs     = ["generic", "packs/net"]
  read_only = ["generated/*.hcl"]
  models    = ["*.hcl", "generated/*.hcl"]
}
