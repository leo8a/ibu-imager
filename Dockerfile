########## Builder ##########
FROM registry.hub.docker.com/library/golang:1.19 AS builder

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


########### Runtime ##########
FROM registry.ci.openshift.org/ocp/4.13:tools

WORKDIR /

COPY --from=builder /workspace/ibu-imager .

ENTRYPOINT ["./ibu-imager"]
