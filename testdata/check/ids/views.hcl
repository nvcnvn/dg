view "main" {}

view "main" {}

view "bad view" {}

view "o" {
  overlay "hull" {
    members = [all]
  }
  overlay "hull" {
    members = [all]
  }
  overlay "bad overlay" {
    members = [all]
  }
}
