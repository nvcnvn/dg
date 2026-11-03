#!/usr/bin/env bash
# Rebuilds examples/state-machine in an empty directory with dg op alone: the states of an
# order, the concurrent regions of one of them, and the order of the regions. Its spec.hcl and
# style.hcl are copied, since operations do not write kinds or styles. DG names the dg binary
# (default dg).
#
#   testdata/rebuild/state-machine.sh <empty directory>
set -euo pipefail
dir=$1
example="$(cd "$(dirname "$0")/../../examples/state-machine" && pwd)"
cp "$example/spec.hcl" "$example/style.hcl" "$dir/"
op() { "${DG:-dg}" op "$1" -C "$dir" "${@:2}" >/dev/null; }

op create_node --id start --kind initial
op create_node --id created --kind simple-state --label 'Created'
op create_node --id processing --kind regions --label 'Processing'
op create_node --id payment --kind region --parent processing --label 'Payment'
op create_node --id pay-start --kind initial --parent payment
op create_node --id authorizing --kind simple-state --parent payment --label 'Authorizing'
op create_node --id paid --kind simple-state --parent payment --label 'Paid'
op create_node --id pay-end --kind final --parent payment
op create_node --id shipping --kind region --parent processing --label 'Shipping'
op create_node --id ship-start --kind initial --parent shipping
op create_node --id picking --kind simple-state --parent shipping --label 'Picking'
op create_node --id packed --kind simple-state --parent shipping --label 'Packed'
op create_node --id ship-end --kind final --parent shipping
op create_node --id delivered --kind simple-state --label 'Delivered'
op create_node --id cancelled --kind simple-state --label 'Cancelled'
op create_node --id end --kind final
op connect --id start-created --from start --to created
op connect --id created-processing --from created --to processing --label 'submit'
op connect --id created-cancelled --from created --to cancelled --label 'cancel'
op connect --id created-edit --from created --to created --label 'edit'
op connect --id pay-start-authorizing --from pay-start --to authorizing
op connect --id authorizing-paid --from authorizing --to paid --label 'authorized'
op connect --id paid-pay-end --from paid --to pay-end
op connect --id ship-start-picking --from ship-start --to picking
op connect --id picking-packed --from picking --to packed --label 'packed'
op connect --id packed-ship-end --from packed --to ship-end
op connect --id processing-delivered --from processing --to delivered --label 'delivered'
op connect --id processing-cancelled --from processing --to cancelled --label 'declined'
op connect --id delivered-end --from delivered --to end
op connect --id cancelled-end --from cancelled --to end

op create_view --id main --title 'Order states'
op reorder --view main --container processing --order payment --order shipping
op set_edge_hint --view main --id created-cancelled --from-side right --via 540,110
