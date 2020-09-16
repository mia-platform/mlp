FROM golang:1.15.2 AS builder

WORKDIR /
COPY go.mod .
COPY go.sum .

RUN go mod download
RUN go mod verify

COPY . .

RUN GOOS=linux CGO_ENABLED=0 GOARCH=amd64 go build -ldflags="-w -s" .

WORKDIR /build

RUN cp -r /miadeploy /LICENSE .

FROM scratch

LABEL name="miadeploy" \
  eu.mia-platform.url="https://www.mia-platform.eu" \
  eu.mia-platform.version="1.0.0"

WORKDIR /

COPY --from=builder /build/* ./

USER 5000

CMD ["/miadeploy"]
