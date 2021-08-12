IMG ?= eu.gcr.io/sap-se-cx-gopher/sap-btp-service-operator-migration:v0.3.0

.PHONY: build-image
build-image:
	docker build --build-arg SSH_PRIVATE_KEY="$$SSH_PRIVATE_KEY" . -t ${IMG}

.PHONY: build-image
push-image:
	docker push ${IMG}

.PHONY: build-cleaner
build-cleaner:
	go build -o cleaner main.go cleaner.go
