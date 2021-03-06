FROM golang:1.15-buster AS build

WORKDIR /src

RUN apt-get update && \
    apt-get install -y libsqlite3-dev libspatialite-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go install ./...

FROM debian:buster

WORKDIR /app

RUN apt-get update && \
    apt-get install -y sqlite3 libsqlite3-0 libspatialite7 ca-certificates

COPY *.html /app/
# COPY static/ *.html /app/static/

COPY --from=build /go/bin/wherewasi /usr/bin/

CMD ["/usr/bin/wherewasi"]
