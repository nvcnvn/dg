project "p" {
  pack "unlocked" {
    source  = "example.com/packs/unlocked"
    version = "v1.0.0"
  }
  pack "moved" {
    source  = "example.com/elsewhere/moved"
    version = "v1.0.0"
  }
  pack "bumped" {
    source  = "example.com/packs/bumped"
    version = "v2.0.0"
  }
  pack "partial" {
    source = "example.com/packs/partial"
  }
  pack "new" {
    version = "v1.0.0"
  }
}
