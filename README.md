# traefik-lazyloader

Takes advantage of traefik's router priority to hit this small application that will manage
starting/stopping containers to save resources.

## Quick-Start

## Config

## Labels

* `lazyloader=true` -- Add to containers that should be managed
* `lazyloader.stopdelay=5m` -- Amount of time to wait for idle network traffick before stopping a container (default: 5m)

# Features

- [ ] Dependencies & groups (eg. shut down DB if all dependent apps are down)

# License

GPLv3
