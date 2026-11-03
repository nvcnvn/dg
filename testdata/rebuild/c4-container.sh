#!/usr/bin/env bash
# Rebuilds examples/c4-container in an empty directory with dg op alone: its model, its views and
# the layout of the pinned view. The vocabulary (dg.hcl and the c4 pack) is copied, since
# operations edit models, views and layouts, not packs. DG names the dg binary (default dg).
#
#   testdata/rebuild/c4-container.sh <empty directory>
set -euo pipefail
dir=$1
example="$(cd "$(dirname "$0")/../../examples/c4-container" && pwd)"
cp "$example/dg.hcl" "$dir/"
cp -R "$example/packs" "$dir/"
op() { "${DG:-dg}" op "$1" -C "$dir" "${@:2}" >/dev/null; }

op create_node --id customer --kind c4-person --label $'Personal Banking Customer\n[Person]\n\nA customer of the bank, with personal bank accounts.'
op create_node --id banking --kind boundary --label $'Internet Banking System\n[Software System]'
op create_node --id web-app --kind app --parent banking --label $'Web Application\n[Container: Java and Spring MVC]\n\nDelivers the static content and the Internet banking single page application.'
op create_node --id spa --kind app --parent banking --label $'Single-Page Application\n[Container: JavaScript and Angular]\n\nProvides all of the Internet banking functionality to customers via their web browser.'
op create_node --id mobile-app --kind app --parent banking --label $'Mobile App\n[Container: Xamarin]\n\nProvides a limited subset of the Internet banking functionality to customers via their mobile device.'
op create_node --id api --kind app --parent banking --label $'API Application\n[Container: Java and Spring MVC]\n\nProvides Internet banking functionality via a JSON/HTTPS API.'
op create_node --id database --kind store --parent banking --label $'Database\n[Container: Oracle Database Schema]\n\nStores user registration information, hashed credentials, access logs, etc.'
op create_node --id email --kind external-system --label $'E-mail System\n[Software System]\n\nThe internal Microsoft Exchange e-mail system.'
op create_node --id mainframe --kind external-system --label $'Mainframe Banking System\n[Software System]\n\nStores all of the core banking information about customers, accounts, transactions, etc.'

op connect --id customer-web --from customer --to web-app --kind uses --label $'Visits bigbank.com/ib\n[HTTPS]'
op connect --id customer-spa --from customer --to spa --kind uses --label $'Views balances,\nmakes payments'
op connect --id customer-mobile --from customer --to mobile-app --kind uses --label $'Views balances,\nmakes payments'
op connect --id web-spa --from web-app --to spa --kind uses --label $'Delivers to the\ncustomer\'s browser'
op connect --id spa-api --from spa --to api --kind uses --label $'API calls\n[JSON/HTTPS]'
op connect --id mobile-api --from mobile-app --to api --kind uses --label $'API calls\n[JSON/HTTPS]'
op connect --id api-database --from api --to database --kind uses --label $'Reads from and writes to\n[SQL/TCP]'
op connect --id api-mainframe --from api --to mainframe --kind uses --label $'Uses\n[XML/HTTPS]'
op connect --id api-email --from api --to email --kind uses --label $'Sends e-mail using\n[SMTP]'
op connect --id email-customer --from email --to customer --kind uses --label 'Sends e-mails to'

op connect --id sign-in-1 --from customer --to spa --kind interaction --label '1. Submits credentials to'
op connect --id sign-in-2 --from spa --to api --kind interaction --label $'2. Sends credentials to\n[JSON/HTTPS]'
op connect --id sign-in-3 --from api --to database --kind interaction --label $'3. Validates credentials\nagainst\n[SQL/TCP]'
op connect --id sign-in-4 --from api --to spa --kind interaction --label '4. Returns a session token'

op create_view --id main --title 'Internet Banking System: containers' --edges edge_kind.uses
op create_view --id pinned --title 'Internet Banking System: containers, partly arranged' --edges edge_kind.uses
op create_view --id sign-in --title 'Internet Banking System: signing in' \
  --include node.customer --include node.banking --include node.spa --include node.api --include node.database \
  --edges edge_kind.interaction

op move --view pinned --id banking --x 0 --y 280
op move --view pinned --id customer --x 400 --y 0
op move --view pinned --id web-app --x 0 --y 0
op move --view pinned --id spa --x 400 --y 0
op move --view pinned --id api --x 400 --y 300
op move --view pinned --id database --x 400 --y 600
