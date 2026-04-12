include help.mk

# get root dir
ROOT_DIR := $(dir $(realpath $(lastword $(MAKEFILE_LIST))))

.DEFAULT_GOAL := start-docker

.PHONY: update
update: ## pulls git repo
	@git -C ${ROOT_DIR} pull
	go mod tidy

.PHONY: test
test: ## run golang test (including integration tests)
	go test ${ROOT_DIR}/...

.PHONY: build
build: ## build goframe binary
	go build ${ROOT_DIR}/...

.PHONY: lint
lint: ## run golangci-lint
	golangci-lint run ${ROOT_DIR}... -E dupl -E gocyclo -E gosec -E misspell -E sqlclosecheck

.PHONY: install-hooks
install-hooks: ## install git hooks
	@echo Installing git hooks...
	@go run -C .githooks install.go

.PHONY: generate
generate: ## regenerate CRD YAML and deepcopy from Go types
	go run sigs.k8s.io/controller-tools/cmd/controller-gen@latest object paths="./internal/operator/api/v1alpha1/..." crd paths="./internal/operator/api/v1alpha1/..." output:crd:dir=./charts/goframe-operator/templates
	@mv ${ROOT_DIR}charts/goframe-operator/templates/goframe.io_goframes.yaml ${ROOT_DIR}charts/goframe-operator/templates/goframes-crd.yaml

.PHONY: generate-check
generate-check: generate ## fail if generated files are out of sync with Go types
	@if ! git diff --exit-code -- internal/operator/api/v1alpha1/zz_generated.deepcopy.go charts/goframe-operator/templates/goframes-crd.yaml > /dev/null 2>&1; then \
		echo "ERROR: generated files are out of sync. Run 'make generate' and commit the result."; \
		git diff -- internal/operator/api/v1alpha1/zz_generated.deepcopy.go charts/goframe-operator/templates/goframes-crd.yaml; \
		exit 1; \
	fi
	@echo "Generated files are up to date."

.PHONY: start-docker
start-docker: ## start goframe server + redis via docker compose
	@docker-compose -f ${ROOT_DIR}docker-compose.yml up --build redis goframe

.PHONY: start-docker-with-image-scheduler
start-docker-with-image-scheduler: ## start goframe, redis, and run image scheduler once
	@docker-compose -f ${ROOT_DIR}docker-compose.yml up --build goframe image-scheduler

.PHONY: run-image-scheduler
run-image-scheduler: ## run the image scheduler once against a running goframe (requires local.yaml)
	@docker-compose -f ${ROOT_DIR}docker-compose.yml run --build --rm image-scheduler

# --- Local K3D development targets ---
IMAGE_NAME := goframe
IMAGE_SCHEDULER_IMAGE_NAME := goframe-image-scheduler
OPERATOR_IMAGE_NAME := goframe-operator
IMAGE_VERSION := latest

.PHONY: start-cluster
start-cluster: ## starts k3d cluster and registry
	@k3d cluster create --config ${ROOT_DIR}k3d/clusterconfig.yaml

.PHONY: stop-k3d
stop-k3d: ## stop K3d
	@k3d cluster delete --config ${ROOT_DIR}k3d/clusterconfig.yaml

.PHONY: restart-k3d
restart-k3d: stop-k3d start-k3d ## restarts K3d

.PHONY: push-k3d
push-k3d: ## build and push server + scheduler images to local k3d registry
	@docker build --build-arg CMD=server ${ROOT_DIR} -t ${IMAGE_NAME}
	@docker tag ${IMAGE_NAME} localhost:5000/${IMAGE_NAME}:${IMAGE_VERSION}
	@docker push localhost:5000/${IMAGE_NAME}:${IMAGE_VERSION}
	@docker build --build-arg CMD=imagescheduler ${ROOT_DIR} -t ${IMAGE_SCHEDULER_IMAGE_NAME}
	@docker tag ${IMAGE_SCHEDULER_IMAGE_NAME} localhost:5000/${IMAGE_SCHEDULER_IMAGE_NAME}:${IMAGE_VERSION}
	@docker push localhost:5000/${IMAGE_SCHEDULER_IMAGE_NAME}:${IMAGE_VERSION}

.PHONY: push-k3d-operator
push-k3d-operator: ## build and push operator image to local k3d registry
	@docker build --build-arg CMD=operator ${ROOT_DIR} -t ${OPERATOR_IMAGE_NAME}
	@docker tag ${OPERATOR_IMAGE_NAME} localhost:5000/${OPERATOR_IMAGE_NAME}:${IMAGE_VERSION}
	@docker push localhost:5000/${OPERATOR_IMAGE_NAME}:${IMAGE_VERSION}

.PHONY: install-operator
install-operator: ## install goframe-operator chart (CRD + operator deployment)
	@helm upgrade --install goframe-operator ${ROOT_DIR}charts/goframe-operator \
		--set image.repository=registry.localhost:5000/${OPERATOR_IMAGE_NAME} \
		--set image.tag=${IMAGE_VERSION} \
		--set image.pullPolicy=Always \
		--set leaderElection.enabled=false

.PHONY: install-redis
install-redis: ## install Redis via OCI chart
	@helm upgrade --install redis oci://registry-1.docker.io/bitnamicharts/redis \
		--version 25.3.9 \
		--set architecture=standalone \
		--set auth.enabled=false \
		--set master.persistence.enabled=false \
		--set "master.resources.requests.cpu=50m" \
		--set "master.resources.requests.memory=64Mi" \
		--set "master.resources.limits.cpu=100m" \
		--set "master.resources.limits.memory=300Mi" \
		--set replica.replicaCount=0 \
		--set sentinel.enabled=false \
		--set metrics.enabled=false \
		--set volumePermissions.enabled=false \
		--set sysctl.enabled=false \
		--set "master.extraFlags[0]=--maxmemory 256mb" \
		--set "master.extraFlags[1]=--maxmemory-policy allkeys-lru" \
		--set "master.extraFlags[2]=--save ''" \
		--set "master.extraFlags[3]=--appendonly no"

.PHONY: start-k3d
start-k3d: start-cluster push-k3d push-k3d-operator install-operator install-redis ## start k3d cluster and deploy operator + GoFrame CR
	@helm upgrade --install ${IMAGE_NAME} ${ROOT_DIR}charts/${IMAGE_NAME} \
		-f ${ROOT_DIR}k3d/values.k3d.yaml \
		--set server.image.repository=registry.localhost:5000/${IMAGE_NAME} \
		--set server.image.tag=${IMAGE_VERSION}

.PHONY: generate-helm-docs
generate-helm-docs: ## re-generates helm docs using docker
	@docker run --rm --volume "$(ROOT_DIR)charts:/helm-docs" jnorwood/helm-docs:latest