########## Builder ##########
FROM registry.hub.docker.com/library/golang:1.19 AS builder

ENV CRIO_VERSION="v1.28.0"

# Set workring directory
WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.sum ./
COPY vendor/ vendor/

# Copy the go source
COPY main.go main.go
COPY cmd/ cmd/

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -mod=vendor -a -o ibu-imager main.go

# Download oc CLI
RUN curl -L https://mirror.openshift.com/pub/openshift-v4/clients/ocp/stable/openshift-client-linux.tar.gz \
        | tar xvzf - -C . && chmod +x ./oc

# Download crio CLI
RUN curl -sL https://github.com/kubernetes-sigs/cri-tools/releases/download/$CRIO_VERSION/crictl-$CRIO_VERSION-linux-amd64.tar.gz \
        | tar xvzf - -C . && chmod +x ./crictl


########### Runtime ##########
FROM registry.access.redhat.com/ubi9/ubi:latest

WORKDIR /

COPY --from=builder /workspace/ibu-imager .
COPY --from=builder /workspace/oc /usr/bin/
COPY --from=builder /workspace/crictl /usr/bin/

RUN yum -y install jq && \
    yum clean all && \
    rm -rf /var/cache/yum

ENTRYPOINT ["./ibu-imager"]