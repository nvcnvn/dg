project "files" {
  packs  = [".", "packs/a"]
  models = ["*.hcl", "*/*.hcl", "packs/*/*.hcl"]
  views  = file("views-file.txt")
}
