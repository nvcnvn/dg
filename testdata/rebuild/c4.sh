#!/usr/bin/env bash
# Rebuilds examples/c4 in an empty directory with dg op alone: its model, its three views with
# their collapses, drill-down targets and view style rules, and the layout of the context view.
# The vocabulary (dg.hcl and the c4 pack) is copied, since operations edit models, views and
# layouts, not packs. DG names the dg binary (default dg).
#
#   testdata/rebuild/c4.sh <empty directory>
set -euo pipefail
dir=$1
example="$(cd "$(dirname "$0")/../../examples/c4" && pwd)"
cp "$example/dg.hcl" "$dir/"
cp -R "$example/packs" "$dir/"
op() { "${DG:-dg}" op "$1" -C "$dir" "${@:2}" >/dev/null; }

op create_node --id customer --kind c4-person --label $'Personal Banking Customer\n[Person]\n\nA customer of the bank, with personal bank accounts.'
op create_node --id banking --kind system --label $'Internet Banking System\n[Software System]\n\nAllows customers to view information about their bank accounts, and make payments.'
op create_node --id mainframe --kind system --label $'Mainframe Banking System\n[Software System]\n\nStores all of the core banking information about customers, accounts, transactions, etc.'
op set_tags --id mainframe --tags existing
op create_node --id email --kind external-system --label $'E-mail System\n[Software System]\n\nThe internal Microsoft Exchange e-mail system.'
op create_node --id web-app --kind app --parent banking --label $'Web Application\n[Container: Java and Spring MVC]\n\nDelivers the static content and the Internet banking single page application.'
op create_node --id spa --kind app --parent banking --label $'Single-Page Application\n[Container: JavaScript and Angular]\n\nProvides all of the Internet banking functionality to customers via their web browser.'
op create_node --id mobile-app --kind app --parent banking --label $'Mobile App\n[Container: Xamarin]\n\nProvides a limited subset of the Internet banking functionality to customers via their mobile device.'
op create_node --id api --kind app-host --parent banking --label $'API Application\n[Container: Java and Spring MVC]\n\nProvides Internet banking functionality via a JSON/HTTPS API.'
op create_node --id database --kind store --parent banking --label $'Database\n[Container: Oracle Database Schema]\n\nStores user registration information, hashed authentication credentials, access logs, etc.'
op create_node --id core --kind app --parent mainframe --label $'Core Banking\n[Container: COBOL]'
op create_node --id ledger --kind store --parent mainframe --label $'Ledger\n[Container: DB2]'
op create_node --id signin --kind component --parent api --label $'Sign In Controller\n[Component: Spring MVC Rest Controller]\n\nAllows users to sign in to the Internet Banking System.'
op create_node --id accounts --kind component --parent api --label $'Accounts Summary Controller\n[Component: Spring MVC Rest Controller]\n\nProvides customers with a summary of their bank accounts.'
op create_node --id reset --kind component --parent api --label $'Reset Password Controller\n[Component: Spring MVC Rest Controller]\n\nAllows users to reset their passwords with a single use URL.'
op create_node --id security --kind component --parent api --label $'Security Component\n[Component: Spring Bean]\n\nProvides functionality related to signing in, changing passwords, etc.'
op create_node --id facade --kind component --parent api --label $'Mainframe Banking System Facade\n[Component: Spring Bean]\n\nA facade onto the mainframe banking system.'
op create_node --id mailer --kind component --parent api --label $'E-mail Component\n[Component: Spring Bean]\n\nSends e-mails to users.'
op connect --id customer-web --from customer --to web-app --kind uses --label $'Visits bigbank.com/ib\n[HTTPS]'
op connect --id customer-spa --from customer --to spa --kind uses --label $'Views account balances,\nand makes payments using'
op connect --id customer-mobile --from customer --to mobile-app --kind uses --label $'Views account balances,\nand makes payments using'
op connect --id web-spa --from web-app --to spa --kind uses --label $'Delivers to the\ncustomer\'s web browser'
op connect --id spa-signin --from spa --to signin --kind uses --label $'Makes API calls to\n[JSON/HTTPS]'
op connect --id spa-accounts --from spa --to accounts --kind uses --label $'Makes API calls to\n[JSON/HTTPS]'
op connect --id spa-reset --from spa --to reset --kind uses --label $'Makes API calls to\n[JSON/HTTPS]'
op connect --id mobile-signin --from mobile-app --to signin --kind uses --label $'Makes API calls to\n[JSON/HTTPS]'
op connect --id mobile-accounts --from mobile-app --to accounts --kind uses --label $'Makes API calls to\n[JSON/HTTPS]'
op connect --id signin-security --from signin --to security --kind uses --label $'Uses'
op connect --id reset-security --from reset --to security --kind uses --label $'Uses'
op connect --id reset-mailer --from reset --to mailer --kind uses --label $'Uses'
op connect --id accounts-facade --from accounts --to facade --kind uses --label $'Uses'
op connect --id security-database --from security --to database --kind uses --label $'Reads from and writes to\n[SQL/TCP]'
op connect --id facade-core --from facade --to core --kind uses --label $'Makes API calls to\n[XML/HTTPS]'
op connect --id core-ledger --from core --to ledger --kind uses
op connect --id mailer-email --from mailer --to email --kind uses --label $'Sends e-mail using'
op connect --id email-customer --from email --to customer --kind uses --label $'Sends e-mails to'

boundary='{"shape": "rect", "fill": "none", "stroke": "#444444", "stroke_dash": [8, 4], "text_color": "#444444", "padding": 20}'
op create_view --id main --title 'Internet Banking System: system context' --collapse kind.system
op create_view --id containers --title 'Internet Banking System: containers' --of banking \
  --include descendants.banking --include node.banking --include node.customer --include node.mainframe --include node.email \
  --collapse node.api --collapse node.mainframe \
  --styles "[{\"label\": \"node.banking\", \"props\": $boundary}]"
op create_view --id components --title 'API Application: components' --of api \
  --include descendants.api --include node.api --include node.spa --include 'node["mobile-app"]' --include node.database \
  --include node.mainframe --include node.email --collapse node.mainframe \
  --styles "[{\"label\": \"node.api\", \"props\": $boundary}]"

op move --view main --id banking --x 250 --y 260
op move --view main --id customer --x 250 --y 0
op move --view main --id email --x 520 --y 540
op move --view main --id mainframe --x 0 --y 540
