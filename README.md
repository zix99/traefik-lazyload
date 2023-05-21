# traefik-lazyloader

Takes advantage of traefik's router priority to proxy to this small application that will manage
starting/stopping containers to save resources.

## Quick-Start

## Config

## Labels

* `lazyloader=true` -- (Required) Add to containers that should be managed
* `lazyloader.stopdelay=5m` -- Amount of time to wait for idle network traffick before stopping a container
* `lazyloader.waitforcode=200` -- Waits for this HTTP result from downstream before redirecting user
* `lazyloader.waitforpath=/`  -- Checks this path downstream to check for the process being ready, using the `waitforcode`

# Features

- [ ] Dependencies & groups (eg. shut down DB if all dependent apps are down)

# License

GPLv3
