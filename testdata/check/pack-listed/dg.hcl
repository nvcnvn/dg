project "p" {
  packs = ["packs/aws", "packs/aws/sub", "packs/AWS", "PACKS/aws/sub"]
  pack "aws" {
    source  = "example.com/packs/aws"
    version = "v1.0.0"
  }
}
