FROM golang:1.14-buster AS build

WORKDIR /src

RUN apt-get update && \
    apt-get install -y libspatialite-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go install ./...

FROM debian:buster

RUN apt-get update && \
    apt-get install -y sqlite3 libspatialite7 ca-certificates

COPY --from=build /go/bin/wherewasi /usr/bin/

CMD ["/usr/bin/wherewasi"]
