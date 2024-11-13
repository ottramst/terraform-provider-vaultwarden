.DEFAULT_GOAL = fmt lint install generate

ACCTEST_PARALLELISM ?= 10
ACCTEST_TIMEOUT = 120m
ACCTEST_COUNT = 1

DOCKER_NETWORK_NAME ?= tf-vaultwarden-network

VAULTWARDEN_DOCKER_NAME ?= tf-vaultwarden
VAULTWARDEN_ENDPOINT ?= http://$(VAULTWARDEN_DOCKER_NAME):8000
VAULTWARDEN_ADMIN_TOKEN ?= admin_token
VAULTWARDEN_VERSION ?= 1.32.4

MAILHOG_DOCKER_NAME ?= tf-vaultwarden-mailhog
MAILHOG_VERSON ?= 1.0.1

SOURCE_LOCATION ?= $(shell pwd)

GOVERSION ?= $(shell grep -e '^go' go.mod | cut -f 2 -d ' ')

.PHONY: build
build:
	go build -v ./...

.PHONY: install
install: build
	go install -v ./...

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: generate
generate:
	cd tools; go generate ./...

.PHONY: fmt
fmt:
	gofmt -s -w -e .

.PHONY: test
test:
	go test -v -cover -timeout=120s -parallel=10 ./...

.PHONY: testacc
testacc:
	TF_ACC=1 go test -v ./... -count $(ACCTEST_COUNT) -parallel $(ACCTEST_PARALLELISM) $(TESTARGS) -timeout $(ACCTEST_TIMEOUT) -cover

# wait_until_healthy command - first argument is the container name
wait_until_healthy = $(call retry, 5, [ "$$(docker inspect -f '{{ .State.Health.Status }}' $(1))" == "healthy" ])

# To run specific test (e.g. TestAccResourceActionConnector) execute `make docker-testacc TESTARGS='-run ^TestAccUserInvite$$'`
# To enable tracing (or debugging), execute `make docker-testacc TF_LOG=TRACE`
.PhONY: docker-testacc
docker-testacc: docker-mailhog docker-vaultwarden
	@ docker run --rm \
		-e VAULTWARDEN_ENDPOINT="$(VAULTWARDEN_ENDPOINT)" \
		-e VAULTWARDEN_ADMIN_TOKEN="$(VAULTWARDEN_ADMIN_TOKEN)" \
		-e TF_LOG="$(TF_LOG)" \
		--network $(DOCKER_NETWORK_NAME) \
		-w "/provider" \
		-v "$(SOURCE_LOCATION):/provider" \
		golang:$(GOVERSION) make testacc TESTARGS="$(TESTARGS)"

.PHONY: docker-mailhog
docker-mailhog: docker-network
	@ docker rm -f $(MAILHOG_DOCKER_NAME) 2> /dev/null || true
	@ docker run -d  \
		-p 8025:8025 \
		-p 1025:1025 \
		--name $(MAILHOG_DOCKER_NAME) \
		--network $(DOCKER_NETWORK_NAME) \
		mailhog/mailhog:v$(MAILHOG_VERSON)

.PHONY: docker-vaultwarden
docker-vaultwarden: docker-network
	@ docker rm -f $(VAULTWARDEN_DOCKER_NAME) 2> /dev/null || true
	@ docker run -d \
    	-p 8000:8000 \
    	-v /tmp/vwdata:/data \
    	-e ROCKET_PORT=8000 \
    	-e ADMIN_TOKEN=$(VAULTWARDEN_ADMIN_TOKEN) \
    	-e SMTP_HOST=$(MAILHOG_DOCKER_NAME) \
    	-e SMTP_PORT=1025 \
    	-e SMTP_SECURITY=off \
    	-e SMTP_FROM=vaultwarden@example.com \
    	-e ADMIN_RATELIMIT_MAX_BURST=10 \
    	-e ADMIN_RATELIMIT_SECONDS=60 \
    	--name $(VAULTWARDEN_DOCKER_NAME) \
    	--network $(DOCKER_NETWORK_NAME) \
    	--health-cmd="curl http://localhost:8000/alive" \
    	--health-interval=10s \
    	vaultwarden/server:$(VAULTWARDEN_VERSION)-alpine
	@ $(call wait_until_healthy, $(ELASTICSEARCH_NAME))

.PHONY: docker-network
docker-network:
	@ docker network inspect $(DOCKER_NETWORK_NAME) > /dev/null 2>&1 || docker network create $(DOCKER_NETWORK_NAME)

.PHONY: docker-clean
docker-clean:
	@ docker rm -f $(VAULTWARDEN_DOCKER_NAME) $(MAILHOG_DOCKER_NAME) || true
	@ docker network rm $(DOCKER_NETWORK_NAME) || true
