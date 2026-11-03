# Generated from the cluster; dg never writes this file (read_only in dg.hcl).

node "kube-system/coredns" {
  kind  = "service"
  label = "CoreDNS"
}

node "kube-system/router:v2" {
  kind  = "service"
  label = "Router v2"
}

edge "dns" {
  from = node["kube-system/router:v2"]
  to   = node["kube-system/coredns"]
}

edge "router-gateway" {
  from  = node["kube-system/router:v2"]
  to    = node.gateway
  label = "gRPC"
}
