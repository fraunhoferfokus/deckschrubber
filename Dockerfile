FROM golang:1.8

WORKDIR /go/src/app
COPY main.go main.go
COPY types.go types.go

RUN go get -d -v ./...
RUN go install -v ./...

ENTRYPOINT ["app"]
