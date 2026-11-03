#!/usr/bin/env bash
# Rebuilds examples/swimlane in an empty directory with dg op alone: the lanes of a lane set and
# the steps in them, the order of the lanes and a view of the lanes alone. DG names the
# dg binary (default dg).
#
#   testdata/rebuild/swimlane.sh <empty directory>
set -euo pipefail
dir=$1
op() { "${DG:-dg}" op "$1" -C "$dir" "${@:2}" >/dev/null; }

op create_node --id fulfilment --kind lane-set --label 'Order fulfilment'
op create_node --id customer --kind lane --parent fulfilment --label 'Customer'
op create_node --id sales --kind lane --parent fulfilment --label 'Sales'
op create_node --id warehouse --kind lane --parent fulfilment --label 'Warehouse'
op create_node --id billing --kind lane --parent fulfilment --label 'Billing'
op create_node --id order --kind terminal --parent customer --label 'Place order'
op create_node --id receive --kind terminal --parent customer --label 'Receive goods'
op create_node --id check --kind node --parent sales --label 'Check order'
op create_node --id in-stock --kind decision --parent sales --label 'In stock?'
op create_node --id backorder --kind node --parent sales --label 'Backorder'
op create_node --id pick --kind node --parent warehouse --label 'Pick items'
op create_node --id ship --kind node --parent warehouse --label 'Ship'
op create_node --id invoice --kind node --parent billing --label 'Send invoice'
op create_node --id payment --kind node --parent billing --label 'Collect payment'
op connect --id order-check --from order --to check
op connect --id check-in-stock --from check --to in-stock
op connect --id in-stock-pick --from in-stock --to pick --label 'yes'
op connect --id in-stock-backorder --from in-stock --to backorder --label 'no'
op connect --id backorder-pick --from backorder --to pick --label 'restocked' --kind async
op connect --id pick-ship --from pick --to ship
op connect --id ship-receive --from ship --to receive
op connect --id ship-invoice --from ship --to invoice
op connect --id invoice-payment --from invoice --to payment

op create_view --id main --title 'Order fulfilment'
op create_view --id lanes --title 'Order fulfilment, lanes only' --of fulfilment
op reorder --view main --container fulfilment --order customer --order sales --order warehouse --order billing
op reorder --view lanes --container fulfilment --order customer --order sales --order warehouse --order billing
