#!/bin/bash
#
# This file defines some bash functions for interactively testing the
# orderservice. To use:
#   $ source repl.sh

echo "OrderService test REPL mode enabled."
echo ""
echo "Commands:"
echo "  $ orderslist      # List orders and pretty-print json response"
echo "  $ ordertake ID    # Take an order, ID must be an integer"
echo "  $ ordercreate     # Create a new order by randomly choosing one of the"
echo "                    # available payloads"

# Show in/out headers, no progress bar.
curlcmd=(curl -v -s --show-error)

# List orders.
function orderslist() {
  ${curlcmd[*]} localhost:8080/orders | jq "."
}

# Take an order, "$ ordertake 3". First arg must be order ID
function ordertake() {
  ${curlcmd[*]} -X PATCH localhost:8080/orders/"$1" --data '{"status":"TAKEN"}'
}

function ordercreate() {
  randompayload=$(find test -type f -name '*.json' | sort --random-sort | head -1)
  echo "Using payload: $(basename "$randompayload")"
  ${curlcmd[*]} --data "@${randompayload}" localhost:8080/orders
}
