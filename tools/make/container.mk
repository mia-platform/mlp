# Copyright Mia srl
# SPDX-License-Identifier: Apache-2.0

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at

#    http://www.apache.org/licenses/LICENSE-2.0

# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

##@ Docker Images Goals

# Force enable buildkit as a build engine
DOCKER_CMD:= DOCKER_BUILDKIT=1 docker
REPO_TAG:= $(shell git describe --tags --exact-match 2>/dev/null || echo latest)
# Making the subst function works with spaces and comas required this hack
COMMA:= ,
EMPTY:=
SPACE:= $(EMPTY) $(EMPTY)
DOCKER_SUPPORTED_PLATFORMS:= $(subst $(SPACE),$(COMMA),$(SUPPORTED_PLATFORMS))
PARSED_TAGS:= $(shell $(TOOLS_DIR)/parse-tags.sh $(REPO_TAG))
IMAGE_TAGS:= $(addprefix --tag , $(foreach REGISTRY, $(CONTAINER_REGISTRIES), $(foreach TAG, $(PARSED_TAGS), $(REGISTRY)/$(CMDNAME):$(TAG))))
CONTAINER_BUILD_DATE:= $(shell date -u "+%Y-%m-%dT%H:%M:%SZ")

DOCKER_LABELS:= --label "org.opencontainers.image.title=$(CMDNAME)"
DOCKER_LABELS+= --label "org.opencontainers.image.description=$(DESCRIPTION)"
DOCKER_LABELS+= --label "org.opencontainers.image.url=$(SOURCE_URL)"
DOCKER_LABELS+= --label "org.opencontainers.image.source=$(SOURCE_URL)"
DOCKER_LABELS+= --label "org.opencontainers.image.version=$(REPO_TAG)"
DOCKER_LABELS+= --label "org.opencontainers.image.created=$(CONTAINER_BUILD_DATE)"
DOCKER_LABELS+= --label "org.opencontainers.image.revision=$(shell git rev-parse HEAD 2>/dev/null)"
DOCKER_LABELS+= --label "org.opencontainers.image.licenses=$(LICENSE)"
DOCKER_LABELS+= --label "org.opencontainers.image.documentation=$(DOCUMENTATION_URL)"
DOCKER_LABELS+= --label "org.opencontainers.image.vendor=$(VENDOR_NAME)"

DOCKER_ANNOTATIONS:= --annotation "org.opencontainers.image.title=$(CMDNAME)"
DOCKER_ANNOTATIONS+= --annotation "org.opencontainers.image.description=$(DESCRIPTION)"
DOCKER_ANNOTATIONS+= --annotation "org.opencontainers.image.url=$(SOURCE_URL)"
DOCKER_ANNOTATIONS+= --annotation "org.opencontainers.image.source=$(SOURCE_URL)"
DOCKER_ANNOTATIONS+= --annotation "org.opencontainers.image.version=$(REPO_TAG)"
DOCKER_ANNOTATIONS+= --annotation "org.opencontainers.image.created=$(CONTAINER_BUILD_DATE)"
DOCKER_ANNOTATIONS+= --annotation "org.opencontainers.image.revision=$(shell git rev-parse HEAD 2>/dev/null)"
DOCKER_ANNOTATIONS+= --annotation "org.opencontainers.image.licenses=$(LICENSE)"
DOCKER_ANNOTATIONS+= --annotation "org.opencontainers.image.documentation=$(DOCUMENTATION_URL)"
DOCKER_ANNOTATIONS+= --annotation "org.opencontainers.image.vendor=$(VENDOR_NAME)"

.PHONY: docker/%/multiarch
docker/%/multiarch:
	$(eval ACTION:= $(word 1,$(subst /, , $*)))
	$(eval IS_PUSH:= $(filter push,$(ACTION)))
	$(eval ADDITIONAL_PARAMETER:= $(if $(IS_PUSH), --push))
	$(info Building image for following platforms: $(SUPPORTED_PLATFORMS) [target: $(DOCKER_TARGET)])
	$(DOCKER_CMD) buildx build --platform "$(DOCKER_SUPPORTED_PLATFORMS)" \
		--build-arg CMD_NAME=$(CMDNAME) \
		--provenance=false \
		$(DOCKER_TARGET_FLAG) \
		$(DOCKER_TARGET_IMAGE_TAGS) \
		$(DOCKER_LABELS) \
		$(DOCKER_ANNOTATIONS) \
		--file ./Dockerfile $(OUTPUT_DIR) $(ADDITIONAL_PARAMETER)

DOCKER_TARGET?= base
DOCKER_TARGET_FLAG= $(if $(DOCKER_TARGET),--target $(DOCKER_TARGET))
DOCKER_TARGET_SUFFIX= $(if $(filter-out base,$(DOCKER_TARGET)),-$(DOCKER_TARGET))
DOCKER_HELM_ALIAS_TAGS= $(if $(filter helm4,$(DOCKER_TARGET)),$(addprefix --tag , $(foreach REGISTRY, $(CONTAINER_REGISTRIES), $(foreach TAG, $(PARSED_TAGS), $(REGISTRY)/$(CMDNAME):$(TAG)-helm))))
DOCKER_TARGET_IMAGE_TAGS= $(addprefix --tag , $(foreach REGISTRY, $(CONTAINER_REGISTRIES), $(foreach TAG, $(PARSED_TAGS), $(REGISTRY)/$(CMDNAME):$(TAG)$(DOCKER_TARGET_SUFFIX)))) $(DOCKER_HELM_ALIAS_TAGS)

.PHONY: docker/build/%
docker/build/%:
	$(eval OS:= $(word 1,$(subst /, ,$*)))
	$(eval ARCH:= $(word 2,$(subst /, ,$*)))
	$(eval ARM:= $(word 3,$(subst /, ,$*)))
	$(info Building image for $(OS) $(ARCH) $(ARM) [target: $(DOCKER_TARGET)])
	$(DOCKER_CMD) build --platform $* \
		--build-arg CMD_NAME=$(CMDNAME) \
		$(DOCKER_TARGET_FLAG) \
		$(DOCKER_TARGET_IMAGE_TAGS) \
		$(DOCKER_LABELS) \
		$(DOCKER_ANNOTATIONS) \
		--file ./Dockerfile $(OUTPUT_DIR)

.PHONY: docker/setup/multiarch
docker/setup/multiarch:
	$(info Setup multiarch emulation...)
	docker run --rm --privileged tonistiigi/binfmt:latest --install $(SUPPORTED_PLATFORMS)

.PHONY: docker/buildx/setup docker/buildx/teardown
docker/buildx/setup:
	docker buildx rm $(BUILDX_CONTEXT) 2>/dev/null || :
	docker buildx create --use --name $(BUILDX_CONTEXT) --platform "$(DOCKER_SUPPORTED_PLATFORMS)"

docker/buildx/teardown:
	docker buildx rm $(BUILDX_CONTEXT)

.PHONY: docker-build
docker-build: go/build/$(DEFAULT_DOCKER_PLATFORM) docker/build/$(DEFAULT_DOCKER_PLATFORM)

.PHONY: docker-build-helm3
docker-build-helm3: DOCKER_TARGET=helm3
docker-build-helm3: docker-build

.PHONY: docker-build-helm4
docker-build-helm4: DOCKER_TARGET=helm4
docker-build-helm4: docker-build

.PHONY: docker-build-helm
docker-build-helm: docker-build-helm4

.PHONY: docker-setup-multiarch
docker-setup-multiarch: docker/setup/multiarch

.PHONY: docker-build-multiarch
docker-build-multiarch: build-multiarch docker/buildx/setup docker/build/multiarch docker/buildx/teardown

.PHONY: docker-build-multiarch-helm3
docker-build-multiarch-helm3: DOCKER_TARGET=helm3
docker-build-multiarch-helm3: docker-build-multiarch

.PHONY: docker-build-multiarch-helm4
docker-build-multiarch-helm4: DOCKER_TARGET=helm4
docker-build-multiarch-helm4: docker-build-multiarch

.PHONY: docker-build-multiarch-helm
docker-build-multiarch-helm: docker-build-multiarch-helm4

.PHONY: ci-docker
ci-docker: docker/push/multiarch ci-docker-helm3 ci-docker-helm4

.PHONY: ci-docker-helm3
ci-docker-helm3: DOCKER_TARGET=helm3
ci-docker-helm3: docker/push/multiarch

.PHONY: ci-docker-helm4
ci-docker-helm4: DOCKER_TARGET=helm4
ci-docker-helm4: docker/push/multiarch
