FROM golang:1.14-buster AS backend

WORKDIR /src

RUN apt-get update && \
    apt-get install -y libsqlite3-dev libspatialite-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go install ./...

FROM node:14-buster as frontend

WORKDIR /src

COPY . .
RUN make orion-index.html

FROM debian:buster

RUN apt-get update && \
    apt-get install -y sqlite3 libsqlite3-0 libspatialite7 ca-certificates

COPY --from=backend /go/bin/wherewasi /usr/bin/
COPY --from=frontend /src/orion-index.html /

CMD ["/usr/bin/wherewasi"]
