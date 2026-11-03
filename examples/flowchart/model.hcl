# A pull request pipeline as a flowchart: steps, decisions, a loop back to the checks, and the
# checks that run in parallel held in a grid.

node "opened" {
  kind  = "terminal"
  label = "Pull request opened"
}

node "checks" {
  kind  = "grid-group"
  label = "Checks"
}

node "lint" {
  kind   = "node"
  label  = "Lint"
  parent = node.checks
}

node "unit" {
  kind   = "node"
  label  = "Unit tests"
  parent = node.checks
}

node "integration" {
  kind   = "node"
  label  = "Integration tests"
  parent = node.checks
}

node "scan" {
  kind   = "node"
  label  = "Security scan"
  parent = node.checks
}

node "green" {
  kind  = "decision"
  label = "All green?"
}

node "fix" {
  kind  = "node"
  label = "Push a fix"
}

node "review" {
  kind  = "node"
  label = "Code review"
}

node "approved" {
  kind  = "decision"
  label = "Approved?"
}

node "merge" {
  kind  = "node"
  label = "Merge"
}

node "deployed" {
  kind  = "terminal"
  label = "Deployed"
}

edge "opened-checks" {
  from = node.opened
  to   = node.checks
}

edge "checks-green" {
  from = node.checks
  to   = node.green
}

edge "green-review" {
  from  = node.green
  to    = node.review
  label = "yes"
}

edge "green-fix" {
  from  = node.green
  to    = node.fix
  label = "no"
}

edge "fix-checks" {
  from  = node.fix
  to    = node.checks
  label = "push"
}

edge "review-approved" {
  from = node.review
  to   = node.approved
}

edge "approved-merge" {
  from  = node.approved
  to    = node.merge
  label = "yes"
}

edge "approved-fix" {
  from  = node.approved
  to    = node.fix
  label = "changes"
}

edge "merge-deployed" {
  from = node.merge
  to   = node.deployed
}
