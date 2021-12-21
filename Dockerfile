################### Build mlp ####################
FROM golang:1.16.10 AS builder

WORKDIR /build

COPY go.mod .
COPY go.sum .

RUN go mod download
RUN go mod verify

ARG version="DEV"
ARG date=""

COPY . .

ENV GO_LDFLAGS="-w -s -X git.tools.mia-platform.eu/platform/devops/deploy/internal/cli.BuildDate=${date} -X git.tools.mia-platform.eu/platform/devops/deploy/internal/cli.Version=${version}"
RUN GOOS=linux \
    CGO_ENABLED=0 \
    GOARCH=amd64 \
    go build -trimpath \
    -ldflags="${GO_LDFLAGS}" \
    -o "mlp" ./cmd/mlp

################## Create image ##################

FROM alpine:3.14

LABEL maintainer="C.E.C.O.M <operations@mia-platform.eu>" \
      name="Image for console deployments" \
      eu.mia-platform.url="https://www.mia-platform.eu" \
      eu.mia-platform.version="3"

COPY --from=builder /build/mlp /usr/local/bin/

# Use an unprivileged user.
USER 1000

CMD ["mlp"]
