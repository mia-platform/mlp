#!/bin/sh
# Mock helm binary for testing - outputs a fixed ConfigMap
case "$1" in
    template)
        cat <<'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: from-helm-chart
data:
  rendered: "true"
EOF
        ;;
    version)
        echo "v3.20.0+mock"
        ;;
    *)
        echo "mock helm: unsupported command $1" >&2
        exit 1
        ;;
esac
