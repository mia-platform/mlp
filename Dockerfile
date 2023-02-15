################### Build mlp ####################
FROM golang:1.20.1 AS builder

WORKDIR /build

COPY go.mod .
COPY go.sum .

RUN go mod download
RUN go mod verify

ARG VERSION="DEV"
ARG BUILDTIME=""

COPY . .

ENV GO_LDFLAGS="-w -s -X github.com/mia-platform/mlp/internal/cli.BuildDate=${BUILDTIME} -X github.com/mia-platform/mlp/internal/cli.Version=${VERSION}"
RUN GOOS=linux \
    CGO_ENABLED=0 \
    GOARCH=amd64 \
    go build -trimpath \
    -ldflags="${GO_LDFLAGS}" \
    -o "mlp" ./cmd/mlp

################## Create image ##################

FROM alpine:3.16.0

COPY --from=builder /build/mlp /usr/local/bin/

CMD ["mlp"]
