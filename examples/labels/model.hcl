node "gateway" {
  label = "Inference gateway\nRoutes requests to a model"
}

node "cluster" {
  kind  = "group"
  label = "GPU cluster\nHanoi data center"
}

node "workers" {
  parent = node.cluster
  label  = "Workers\nServe the models"
}

node "storage" {
  parent = node.cluster
  label  = "Model storage\nHolds the weights"
}

edge "requests" {
  from = node.gateway
  to   = node.workers
}

edge "weights" {
  from = node.storage
  to   = node.workers
}
