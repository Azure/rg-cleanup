IMAGE_REGISTY ?= chewong
IMAGE_NAME := rg-cleanup
IMAGE_VERSION ?= v0.1.1

.PHONY: all
all: build

.PHONY: build
build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/rg-cleanup ./main.go

.PHONY: test
test:
	go test -v ./...

.PHONY: image
image: build
	docker build -t $(IMAGE_REGISTY)/$(IMAGE_NAME):$(IMAGE_VERSION) .

.PHONY: push
push:
	docker push $(IMAGE_REGISTY)/$(IMAGE_NAME):$(IMAGE_VERSION)

.PHONY: clean
clean:
	rm -rf bin
