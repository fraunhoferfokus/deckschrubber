FROM golang:1.13 AS builder
WORKDIR /go/src/app
COPY . .
RUN go build

FROM alpine:3.11
RUN apk add --update --no-cache ca-certificates libc6-compat
COPY --from=builder /go/src/app/deckschrubber /
ENTRYPOINT ["/deckschrubber"]
