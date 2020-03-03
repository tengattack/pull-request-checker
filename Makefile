NAME=unified-ci
VERSION=0.1.6
REGISTRY_PREFIX=$(if $(REGISTRY),$(addsuffix /, $(REGISTRY)))

.PHONY: build publish version

build:
	docker build --rm --build-arg version=${VERSION} --build-arg proxy= -t ${NAME}:${VERSION} .

publish:
	docker tag ${NAME}:${VERSION} ${REGISTRY_PREFIX}${NAME}:${VERSION}
	docker push ${REGISTRY_PREFIX}${NAME}:${VERSION}

version:
	@echo ${VERSION}

