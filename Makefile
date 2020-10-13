BUILD_FILES = $(shell go list -f '{{range .GoFiles}}{{$$.Dir}}/{{.}}\
{{end}}' ./...)
MLP_VERSION ?= $(shell git describe --tags 2>/dev/null || git rev-parse --short HEAD)
DATE_FMT = +%Y-%m-%d
ifdef SOURCE_DATE_EPOCH
    BUILD_DATE ?= $(shell date -u -d "@$(SOURCE_DATE_EPOCH)" "$(DATE_FMT)" 2>/dev/null || date -u -r "$(SOURCE_DATE_EPOCH)" "$(DATE_FMT)" 2>/dev/null || date -u "$(DATE_FMT)")
else
    BUILD_DATE ?= $(shell date "$(DATE_FMT)")
endif

ifndef CGO_CPPFLAGS
    export CGO_CPPFLAGS := $(CPPFLAGS)
endif
ifndef CGO_CFLAGS
    export CGO_CFLAGS := $(CFLAGS)
endif
ifndef CGO_LDFLAGS
    export CGO_LDFLAGS := $(LDFLAGS)
endif

GO_LDFLAGS := -X git.tools.mia-platform.eu/platform/devops/deploy/internal/cli.Version=$(MLP_VERSION) $(GO_LDFLAGS)
GO_LDFLAGS := -X git.tools.mia-platform.eu/platform/devops/deploy/internal/cli.BuildDate=$(BUILD_DATE) $(GO_LDFLAGS)

.PHONY: all build test clean

all: clean coverage build

build: $(BUILD_FILES)
	go build -trimpath -ldflags "$(GO_LDFLAGS)" -o "mlp" ./cmd/mlp

test:
	go test -race ./... --coverprofile=coverage.out

coverage: test
	go tool cover -func=coverage.out

clean:
	rm -rf ./mlp
	go clean -cache
	rm -rf *.out
