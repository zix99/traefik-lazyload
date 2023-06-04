# BUILD
FROM golang:1.18-alpine3.17 AS build

WORKDIR /opt/src
COPY go.* ./
RUN go mod download

COPY . .
RUN go build .

# Make the final image
FROM alpine:latest

EXPOSE 8080
WORKDIR /opt/app
COPY --from=build /opt/src/traefik-lazyload .
COPY config.yaml .

CMD ./traefik-lazyload
