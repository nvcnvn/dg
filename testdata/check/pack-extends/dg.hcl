project "p" {
  pack "aws" {
    source  = "example.com/packs/aws"
    version = "v1.0.0"
  }
  pack "gcp" {
    source  = "example.com/packs/gcp"
    version = "v1.0.0"
  }
}
