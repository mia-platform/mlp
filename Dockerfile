# syntax=docker/dockerfile:1
FROM docker.io/library/alpine:3.23.4@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11 AS base

ARG TARGETPLATFORM
ARG CMD_NAME
ENV COMMAND_NAME=${CMD_NAME}

COPY ${TARGETPLATFORM}/${CMD_NAME} /usr/local/bin/

CMD ["/bin/sh", "-c", "${COMMAND_NAME}"]

FROM base AS helm3

ARG TARGETPLATFORM
ARG HELM3_VERSION=3.20.0
RUN <<'EOF'
  set -eu
  case "${TARGETPLATFORM}" in
    linux/amd64)  ARCH=amd64 ;;
    linux/arm64)  ARCH=arm64 ;;
    linux/arm/v6) ARCH=arm   ;;
    linux/arm/v7) ARCH=arm   ;;
    linux/386)    ARCH=386   ;;
    *) echo "unsupported platform: ${TARGETPLATFORM}" >&2; exit 1 ;;
  esac
  wget -qO- "https://get.helm.sh/helm-v${HELM3_VERSION}-linux-${ARCH}.tar.gz" \
    | tar xz --strip-components=1 -C /usr/local/bin "linux-${ARCH}/helm"
  helm version
EOF

FROM base AS helm4

ARG TARGETPLATFORM
ARG HELM4_VERSION=4.1.1
RUN <<'EOF'
  set -eu
  case "${TARGETPLATFORM}" in
    linux/amd64)  ARCH=amd64 ;;
    linux/arm64)  ARCH=arm64 ;;
    linux/arm/v6) ARCH=arm   ;;
    linux/arm/v7) ARCH=arm   ;;
    linux/386)    ARCH=386   ;;
    *) echo "unsupported platform: ${TARGETPLATFORM}" >&2; exit 1 ;;
  esac
  wget -qO- "https://get.helm.sh/helm-v${HELM4_VERSION}-linux-${ARCH}.tar.gz" \
    | tar xz --strip-components=1 -C /usr/local/bin "linux-${ARCH}/helm"
  helm version
EOF
