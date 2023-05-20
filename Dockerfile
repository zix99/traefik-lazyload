FROM golang:1.18

EXPOSE 8080
WORKDIR /opt/app
COPY go.* .
RUN go mod download

COPY . .
RUN go build .

CMD ./traefik-lazyload
