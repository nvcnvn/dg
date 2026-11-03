#!/usr/bin/env bash
# Rebuilds examples/incident in an empty directory with dg op alone: its model, its views with
# the overlay, the collapse and the view style rules, and the incident view's layout with its
# decorations. The project has no dg.hcl; its style.hcl (an overlay
# style) and the alert icon are copied, since operations do not write styles or pictures. DG
# names the dg binary (default dg).
#
#   testdata/rebuild/incident.sh <empty directory>
set -euo pipefail
dir=$1
example="$(cd "$(dirname "$0")/../../examples/incident" && pwd)"
cp "$example/style.hcl" "$dir/"
cp -R "$example/icons" "$dir/"
op() { "${DG:-dg}" op "$1" -C "$dir" "${@:2}" >/dev/null; }

op create_node --id edge --kind group --label $'Edge'
op create_node --id lb --kind service --parent edge --label $'Load balancer'
op create_node --id gateway --kind service --parent edge --label $'API gateway'
op create_node --id core --kind group --label $'Core services'
op create_node --id orders --kind service --parent core --label $'Orders'
op create_node --id payments --kind service --parent core --label $'Payments'
op set_tags --id payments --tags degraded
op create_node --id inventory --kind service --parent core --label $'Inventory'
op create_node --id events --kind queue --label $'Order events'
op create_node --id data --kind group --label $'Data'
op create_node --id orders-db --kind database --parent data --label $'Orders DB'
op create_node --id payments-db --kind database --parent data --label $'Payments DB'
op set_tags --id payments-db --tags degraded
op create_node --id cache --kind database --parent data --label $'Stock cache'
op create_node --id psp --kind external --label $'Payment provider'
op set_tags --id psp --tags degraded
op connect --id lb-gateway --from lb --to gateway
op connect --id gateway-orders --from gateway --to orders --label $'checkout'
op connect --id orders-payments --from orders --to payments --label $'charge'
op connect --id orders-inventory --from orders --to inventory --label $'reserve'
op connect --id orders-db --from orders --to orders-db
op connect --id payments-db --from payments --to payments-db
op connect --id inventory-cache --from inventory --to cache
op connect --id payments-psp --from payments --to psp --label $'authorize'
op connect --id orders-events --from orders --to events --kind async
op connect --id payments-events --from payments --to events --kind async

op create_view --id main --title 'Checkout platform'
op create_view --id incident --title 'Incident: payment provider timeouts' --collapse node.data \
  --overlays '[{"id": "blast-radius", "members": ["tag.degraded"], "style": "hull-red", "label": "Blast radius"}]' \
  --styles '[{"label": "tag.degraded", "props": {"fill": "#FCE8E6", "stroke": "#D93025"}}, {"label": "node.payments", "props": {"stroke_width": 3}}, {"type": "edge_style", "label": "edge.payments-psp", "props": {"stroke": "#D93025", "animate": "flow"}}]'

op move --view incident --id core --x 120 --y 290
op move --view incident --id data --x 300 --y 590
op move --view incident --id edge --x 140 --y 10
op move --view incident --id events --x 90 --y 590
op move --view incident --id gateway --x 10 --y 100
op move --view incident --id inventory --x 20 --y 100
op move --view incident --id lb --x 0 --y 0
op move --view incident --id orders --x 30 --y 0
op move --view incident --id payments --x 160 --y 100
op move --view incident --id psp --x 520 --y 440

deco() { op add_decoration --view incident --id "$1" --decoration "$2"; }
deco banner '{"type": "band", "x": -20, "y": -90, "w": 800, "h": 70, "fill": "#FFF4E5"}'
deco alert '{"type": "image", "x": 0, "y": -70, "w": 30, "h": 30, "src": "icons/alert.svg"}'
deco title '{"type": "text", "x": 40, "y": -70, "text": "Payment provider timeouts, 14:02 UTC", "font_size": 20}'
deco underline '{"type": "freehand", "x": 40, "y": -40, "path": "M0 0 C 120 6 240 -6 360 0", "stroke": "#D93025", "stroke_width": 2}'
deco legend '{"type": "rect", "x": 520, "y": 10, "w": 230, "h": 90, "fill": "#FFFFFF", "stroke": "#DADCE0"}'
deco legend-title '{"type": "text", "x": 530, "y": 20, "text": "Legend", "text_color": "#5F6368", "font_size": 12}'
deco legend-degraded '{"type": "text", "x": 530, "y": 50, "text": "Red: degraded", "font_size": 12}'
deco legend-hull '{"type": "text", "x": 530, "y": 70, "text": "Shaded outline: blast radius", "font_size": 12}'
deco cause '{"type": "ellipse", "attach": "psp", "w": 200, "h": 90, "stroke": "#D93025", "stroke_dash": [4, 3]}'
deco cause-arrow '{"type": "arrow", "attach": "psp", "points": [[0, 80], [0, 50]], "stroke": "#D93025"}'
deco cause-note '{"type": "text", "attach": "psp", "dx": 0, "dy": 90, "text": "Root cause: provider 5xx", "text_color": "#D93025", "font_size": 12}'
