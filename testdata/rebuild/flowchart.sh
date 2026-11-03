#!/usr/bin/env bash
# Rebuilds examples/flowchart in an empty directory with dg op alone: the steps and decisions of
# a pull request pipeline with a loop back to its checks, its checks in a grid, and the view rule that gives the grid two
# centered columns. DG names the dg binary (default dg).
#
#   testdata/rebuild/flowchart.sh <empty directory>
set -euo pipefail
dir=$1
op() { "${DG:-dg}" op "$1" -C "$dir" "${@:2}" >/dev/null; }

op create_node --id opened --kind terminal --label 'Pull request opened'
op create_node --id checks --kind grid-group --label 'Checks'
op create_node --id lint --kind node --parent checks --label 'Lint'
op create_node --id unit --kind node --parent checks --label 'Unit tests'
op create_node --id integration --kind node --parent checks --label 'Integration tests'
op create_node --id scan --kind node --parent checks --label 'Security scan'
op create_node --id green --kind decision --label 'All green?'
op create_node --id fix --kind node --label 'Push a fix'
op create_node --id review --kind node --label 'Code review'
op create_node --id approved --kind decision --label 'Approved?'
op create_node --id merge --kind node --label 'Merge'
op create_node --id deployed --kind terminal --label 'Deployed'
op connect --id opened-checks --from opened --to checks
op connect --id checks-green --from checks --to green
op connect --id green-review --from green --to review --label 'yes'
op connect --id green-fix --from green --to fix --label 'no'
op connect --id fix-checks --from fix --to checks --label 'push'
op connect --id review-approved --from review --to approved
op connect --id approved-merge --from approved --to merge --label 'yes'
op connect --id approved-fix --from approved --to fix --label 'changes'
op connect --id merge-deployed --from merge --to deployed

op create_view --id main --title 'Pull request pipeline' \
  --styles '[{"label": "node.checks", "props": {"columns": 2, "align": "center", "gap": 20}}]'
op reorder --view main --container checks --order lint --order unit --order scan --order integration
op set_edge_hint --view main --id fix-checks --from-side bottom --to-side left
