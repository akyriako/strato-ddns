# Configuration variables
REGISTRY ?= $(shell docker info | sed '/Username:/!d;s/.* //')
IMAGE_NAME ?= strato-go-dyndns
TAG ?= 0.2.0
DOCKERFILE ?= Dockerfile

# Build binary
build:
	@echo "Building Go binary..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/strato-go-dyndns .

# Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t $(REGISTRY)/$(IMAGE_NAME):$(TAG) -t $(REGISTRY)/$(IMAGE_NAME):latest -f $(DOCKERFILE) .

# Push Docker image
docker-push:
	@echo "Pushing Docker image to registry..."
	docker push $(REGISTRY)/$(IMAGE_NAME):$(TAG)
	docker push $(REGISTRY)/$(IMAGE_NAME):latest

# Clean up
clean:
	@echo "Cleaning up..."
	rm -f bin/strato-go-dyndns

# Default target
.PHONY: build docker-build docker-push clean
