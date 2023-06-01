# traefik-lazyloader

This small app will automatically start and stop docker containers based on access, for instance,
via traefik.

## How it Works

It works by acting as the fallback-route for the containers. For instance, if you have
`example.com` as a container you want to lazy-load, you would add the container, as well
as this lazyloader that would act as a lower-priority router for the same domain. If the
host is accessed, the lazyloader will work to boot up the container and redirect the user
as soon as it's up.

It then monitors the container's network interface. If the network is idle for X minutes, it
will stop the container.

## Quick-Start

### docker-compose
```yaml
version: '3.5'

services:
  # Example traefik proxy
  reverse-proxy:
    image: traefik:v2.4
    command:
      - --api.insecure
      - --providers.docker
      - --providers.docker.defaultRule=Host(`{{.Name}}.example.com`)
      - --entryPoints.web.address=:80
      - --entryPoints.web.forwardedHeaders.insecure
      - --providers.docker.exposedByDefault=false
    restart: always
    ports:
      - "80:80"     # The HTTP port
      - "8080:8080" # The Web UI (enabled by --api)
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock # So that Traefik can listen to the Docker events

  # Lazy-loader manager
  lazyloader:
    build: .
    labels:
      - traefik.enable=true
      - "traefik.http.routers.lazyload.priority=-100" # Lower router priority. Would only be hit if the app isn't running
      - "traefik.http.routers.lazyload.rule=Host(`whoami.example.com`, `lazyloader.example.com`)"
    environment:
      TLL_STOPATBOOT: true                   # Stop all lazyloaded containers at boot (great for an example)
      TLL_STATUSHOST: lazyloader.example.com # This hostname will display a status page. Disabled by default
    networks:
      - traefik-bridge
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock # Must access docker

  whoami:
    image: containous/whoami
    networks:
      - traefik-bridge
    labels:
      - traefik.enable=true
      - "traefik.http.routers.lazywhoami.rule=Host(`whoami.example.com`)"
      - lazyloader=true
      - lazyloader.stopdelay=30s # Overrides the default

networks:
  traefik-bridge:
    external: true
    name: traefik-bridge
```

You can run `docker-compose up` on the above for a quick-start. You will need to alter the domains as needed.

## Config

Configuration uses [viper]() and can be specified by either overwriting the `config.yaml` file or
via environment variables with the `TLL_` prefix (Traefik lazy loader)

```yaml
# What port to listen on
listen: :8080

# If set, when access via this hostname, will display status page
statushost: ""

# Enable debug logging
verbose: false

# if true, will stop all running tagged containers when the lazyloader starts
stopatboot: false

# which splash-page asset to use
splash: splash.html

# Container defaults
stopdelay: 5m # How long to wait before stopping container
pollfreq: 10s # How often to check

# This will be the label-prefix to look at settings on a container
# usually won't need to change (only if running multiple instances)
labelprefix: lazyloader
```

## Labels

* `lazyloader=true` -- (Required) Add to containers that should be managed
* `lazyloader.stopdelay=5m` -- Amount of time to wait for idle network traffick before stopping a container
* `lazyloader.waitforcode=200` -- Waits for this HTTP result from downstream before redirecting user. Can be comma-separated list
* `lazyloader.waitforpath=/`  -- Checks this path downstream to check for the process being ready, using the `waitforcode`
* `lazyloader.waitformethod=HEAD` -- Method to check against the downstream server
* `lazyloader.hosts=a.com,b.net,etc` -- Set specific hostnames that will trigger. By default, will look for traefik router

### Dependencies

* `lazyloader.needs=a,b,c` -- List of dependencies a container needs (will be started before starting the container). Can only be specified on a `lazyloader=true` container
* `lazyloader.provides=a` -- What dependency name a container provides (Not necessarily a `lazyloader` container)
* `lazyloader.provides.delay=5s` -- Delay starting other containers for this duration

# License

GPLv3
