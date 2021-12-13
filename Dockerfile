################### Build mlp ####################
FROM golang:1.16.10 AS mlp

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

############ Install Helm and kubectl ############

FROM alpine:3.14 AS tools

ENV K8S_VERSION="v1.20.2"
ENV HELM_VERSION="v3.5.2"

WORKDIR /build

RUN wget https://storage.googleapis.com/kubernetes-release/release/${K8S_VERSION}/bin/linux/amd64/kubectl && \
  wget https://get.helm.sh/helm-${HELM_VERSION}-linux-amd64.tar.gz && \
  tar xf helm-${HELM_VERSION}-linux-amd64.tar.gz \
  && mv linux-amd64/helm . \
  && rm -fr linux-amd64 helm-${HELM_VERSION}-linux-amd64.tar.gz && \
  chmod +x kubectl helm

################## Create image ##################

FROM alpine:3.14

LABEL maintainer="C.E.C.O.M <operations@mia-platform.eu>" \
      name="Image for console deployments" \
      eu.mia-platform.url="https://www.mia-platform.eu" \
      eu.mia-platform.version="3"

RUN apk add --no-cache make curl

COPY --from=tools /build/* /usr/local/bin/
COPY --from=mlp /build/mlp /usr/local/bin/

CMD ["mlp"]
