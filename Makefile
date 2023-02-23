SHELL=/usr/bin/env bash

TAG?=$(shell date +%F)-$(shell git describe --always --tag --dirty)
REPO?=147263665150.dkr.ecr.eu-west-1.amazonaws.com
REPO_USER?=AWS
REPO_REGION?=eu-west-1

.PHONY: all
all: build-all

.PHONY: push
push: push-all

.PHONY: build-all
build-all: build-dealgood build-ironbar build-skyfish

.PHONY: build-dealgood
build-dealgood:
	docker build -f Dockerfile-dealgood -t dealgood:${TAG} .

.PHONY: build-ironbar
build-ironbar:
	docker build -f Dockerfile-skyfish -t skyfish:${TAG} .

.PHONY: build-skyfish
build-skyfish:
	docker build -f Dockerfile-skyfish -t skyfish:${TAG} .

.PHONY: push-all
push-all: push-dealgood push-ironbar push-skyfish

.PHONY: push-dealgood
push-dealgood: build-dealgood docker-login
	docker tag dealgood:${TAG} ${REPO}/dealgood:${TAG}
	docker push ${REPO}/dealgood:${TAG}

.PHONY: push-ironbar
push-ironbar: build-ironbar docker-login
	docker tag ironbar:${TAG} ${REPO}/ironbar:${TAG}
	docker push ${REPO}/ironbar:${TAG}

.PHONY: push-skyfish
push-skyfish: build-skyfish docker-login
	docker tag skyfish:${TAG} ${REPO}/skyfish:${TAG}
	docker push ${REPO}/skyfish:${TAG}

.PHONY: docker-login
docker-login:
	aws ecr get-login-password --region ${REPO_REGION} | docker login --username ${REPO_USER} --password-stdin ${REPO}

