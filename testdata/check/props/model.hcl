node "w1" {
  kind  = "gpu"
  props = { gpu_count = "eight", vendor = "nv", color = "red" }
}

node "w2" {
  kind  = "gpu"
  props = { gpu_count = 4, gpu_count = 5 }
}

node "w3" {
  props = "gpu_count"
}
