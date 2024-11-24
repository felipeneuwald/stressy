include .env

default: help

## build: build binaries
build:
	@export CGO_ENABLED=0
	@os='linux darwin freebsd netbsd openbsd'; for k in $$os; do env GOOS=$$k GOARCH=amd64 go build -o bin/$(PROJECTNAME)_$$k *.go ; done

## docker_build: build docker image
docker_build:
	docker build -t $(DOCKERHUB_USER)/$(DOCKERHUB_REPO) .

## docker_push: push docker image to registry
docker_push:
	docker push $(DOCKERHUB_USER)/$(DOCKERHUB_REPO)

## release: create a new release using goreleaser
release:
	goreleaser release --clean

## release-snapshot: test the release process without publishing
release-snapshot:
	goreleaser release --snapshot --clean --skip-publish

help: Makefile
	@echo " Choose a command to run in "$(PROJECTNAME)":"
	@echo
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'
	@echo
