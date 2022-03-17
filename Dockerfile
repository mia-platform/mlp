################### Build mlp ####################
FROM golang:1.17.8 AS builder

WORKDIR /build

COPY go.mod .
COPY go.sum .

RUN go mod download
RUN go mod verify

ARG version="DEV"
ARG date=""

COPY . .

ENV GO_LDFLAGS="-w -s -X github.com/mia-platform/mlp/internal/cli.BuildDate=${date} -X github.com/mia-platform/mlp/internal/cli.Version=${version}"
RUN GOOS=linux \
    CGO_ENABLED=0 \
    GOARCH=amd64 \
    go build -trimpath \
    -ldflags="${GO_LDFLAGS}" \
    -o "mlp" ./cmd/mlp

################## Create image ##################

FROM alpine:3.15

ARG version="DEV"

LABEL maintainer="C.E.C.O.M <operations@mia-platform.eu>" \
      name="mlp: cli for easier deployment of Mia-Platform Console projects" \
      eu.mia-platform.url="https://www.mia-platform.eu" \
      eu.mia-platform.version="${version}"

COPY --from=builder /build/mlp /usr/local/bin/

CMD ["mlp"]
