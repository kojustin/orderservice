# OrderService

This repository contains the source code for the OrderService which manages
orders and exposes them as an HTTP service. *To run it:*

    GOOGLE_MAPS_API_KEY=XXXXX ./start.sh

The application needs a Google Maps key to be able to perform requests. When
running the application, set the following environment variable to the value of
the Google Maps Cloud Platform API Key.

`GOOGLE_MAPS_API_KEY`: Google Maps Cloud Platform API Key, suitable for use
with the [distance matrix API][matrixapi].

[matrixapi]: https://developers.google.com/maps/documentation/distance-matrix/web-service-best-practices#BuildingURLs

## Local Development

There is a `Vagrantfile` included for convenience. When working with Vagrant,
first bring up a VM:

    vagrant up

Then provision, build, and "deploy" the service to the VM:

    vagrant ssh --command "cd /vagrant && GOOGLE_MAPS_API_KEY=XXXXX ./start.sh"

`start.sh` is idempotent so it is safe to run at any time.

Once inside the VM:

    cd /vagrant && make artifacts/containerize/orderservice && artifacts/containerize/orderservice

#### Caesar Says

Ynynzbir Punyyratr in Latin is ...
