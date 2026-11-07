project "ops" {
  read_only = ["generated.hcl"]

  pack "aws" {
    source  = "example.com/aws"
    version = "v1.0.0"
  }
}
