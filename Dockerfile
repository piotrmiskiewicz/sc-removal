# Builder image
FROM golang:1.16 as builder

# Workaround because migration tool is private repo
ARG SSH_PRIVATE_KEY
RUN mkdir /root/.ssh/ && \
    echo "${SSH_PRIVATE_KEY}" > /root/.ssh/id_rsa && \
    chmod 600 /root/.ssh/id_rsa && \
    ssh-keyscan -t rsa -H github.com > ~/.ssh/known_hosts

ENV GOPRIVATE=github.com/SAP/sap-btp-service-operator-migration
RUN git config --global url.git@github.com:.insteadOf https://github.com/

RUN git clone --depth=1 https://github.com/SAP/sap-btp-service-operator-migration && \
    cd sap-btp-service-operator-migration && \
    CGO_ENABLED=0 go build -o /go/bin/sap-btp-service-operator-migration main.go

# build cleaner
WORKDIR /go/src/github.com/kyma-incubator/sc-removal

COPY go.mod go.mod
COPY go.sum go.sum
COPY main.go main.go
COPY cleaner.go cleaner.go
COPY deploy/run.sh /run.sh

RUN CGO_ENABLED=0 go build -o /go/bin/cleaner main.go cleaner.go

# Run image
FROM alpine:3.14.0

COPY --from=builder /etc/ssl/certs /etc/ssl/certs
COPY --from=builder /go/bin/sap-btp-service-operator-migration /bin/sap-btp-service-operator-migration
COPY --from=builder /go/bin/cleaner /bin/cleaner
COPY --from=builder /run.sh /bin/run.sh
RUN chmod a+x /bin/run.sh

ENTRYPOINT ["run.sh"]
