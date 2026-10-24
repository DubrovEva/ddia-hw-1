FROM golang:1.25.14-alpine3.24 AS build

ARG CMD

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/service ./cmd/${CMD}

FROM alpine:3.24.1

RUN adduser -D -u 10001 service
USER service
COPY --from=build /out/service /usr/local/bin/service

ENTRYPOINT ["/usr/local/bin/service"]
