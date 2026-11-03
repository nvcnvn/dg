node "gateway" {
  kind  = "service"
  label = "Inference Gateway"
}

node "pool" {
  kind  = "group"
  label = "GPU Pool"
}

node "worker-1" {
  kind   = "gpu-node"
  label  = "Worker 1"
  parent = node.pool
  tags   = ["critical"]
  props  = { gpu_count = 8 }
}

node "worker-2" {
  kind   = "gpu-node"
  label  = "Worker 2"
  parent = node.pool
  props  = { gpu_count = 4 }
}

node "tor-1" {
  kind  = "switch"
  label = "ToR 1"
  ports = port_range("p", 1, 8)
  props = { speed = "400G" }
}

node "weights" {
  kind  = "storage"
  label = "Model weights"
  port "nvme" {
    side = "left"
  }
}

edge "gw-w1" {
  from = node.gateway
  to   = node.worker-1
}

edge "gw-w2" {
  from = node.gateway
  to   = node.worker-2
}

edge "w1-tor" {
  from  = node.worker-1.port.nic0
  to    = node.tor-1.port.p1
  kind  = "rdma"
  label = "400G"
}

edge "w2-tor" {
  from  = node.worker-2.port.nic0
  to    = node.tor-1.port.p2
  kind  = "rdma"
  label = "400G"
}

edge "tor-weights" {
  from = node.tor-1.port.p8
  to   = node.weights.port.nvme
  kind = "rdma"
}
