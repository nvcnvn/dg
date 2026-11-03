# Characters of several code points before diagnostics, tabs, and CRLF line ends.
node "🇻🇳" {
	label = "👩‍💻 🇻🇳 é"
	tags  = ["éé", "🇻🇳", "👩‍💻", "x y"]
}

edge "e" {
	from = node["🇻🇳"]
	to   = node["éé"]
}
