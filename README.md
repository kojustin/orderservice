# OrderService

This repository contains the source code for the OrderService which manages
orders and exposes an API as an HTTP service. **Run it like this:**

    GOOGLE_MAPS_API_KEY=XXXXX ./start.sh

`start.sh` is idempotent.

The application needs a Google Maps key to be able to perform requests. When
running the application, set the environment variable GOOGLE_MAPS_API_KEY to
the value of the Google Maps Cloud Platform API Key, suitable for use
with the [distance matrix API][matrixapi].

[matrixapi]: https://developers.google.com/maps/documentation/distance-matrix/web-service-best-practices#BuildingURLs

## Local Development

For convenience of local development, a `Vagrantfile` is included to simulate
the necessary cloud environment. When working with Vagrant, first start the VM:

    vagrant up

Then provision, build, and "deploy" the service inside the VM:

    vagrant ssh --command "cd /vagrant && GOOGLE_MAPS_API_KEY=XXXXX ./start.sh"

To iterate quickly:

    cd /vagrant && \
        make artifacts/containerize/orderservice artifacts/orders.db && \
        artifacts/containerize/orderservice -dbpath artifacts/orders.db

## Interactive Tests

Add interactive test functions to your bash shell.

    source test/repl.sh

#### Caesar Says

Ynynzbir Punyyratr in Latin is ...
