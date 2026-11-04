view "main" {
  title = "What the system does"
}

view "sizing" {
  title = "How large it is"

  node "gateway" {
    label = "Inference gateway\n3 replicas, 2 vCPU each"
  }
  node "workers" {
    label = "Workers\n8 nodes, 8 GPUs each"
  }
  node "storage" {
    label = "Model storage\n200 TB NVMe"
  }
}
