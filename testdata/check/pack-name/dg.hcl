project "p" {
  pack "generic" {
    source  = "example.com/packs/generic"
    version = "v1.0.0"
  }
  pack "bad.name" {
    source  = "example.com/packs/bad.name"
    version = "v1.0.0"
  }
  pack "aws" {
    source  = "example.com/packs/aws"
    version = "v1.0.0"
  }
  pack "aws" {
    source  = "example.com/packs/aws"
    version = "v1.0.0"
  }
  pack "AWS" {
    source  = "example.com/packs/AWS"
    version = "v1.0.0"
  }
}
