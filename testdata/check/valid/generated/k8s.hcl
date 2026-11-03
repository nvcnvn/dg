node "kube-system/coredns" {
  tags = ["generated"]
}

edge "dns" {
  from = node["kube-system/coredns"]
  to   = node["tor-1"].port.uplink
}
