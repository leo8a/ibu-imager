# IBU Imager

This application will assist user to easily create an OCI seed image for the Image-Based Upgrade (IBU) workflow, using 
a simple CLI.

## Motivation

One of the most important / critical day2 operations for Telecommunications cloud-native deployments is upgrading their
Container-as-a-Service (CaaS) platforms as quick (and secure!) as possible while minimizing the disrupting time for 
active workloads.

A novel method to approach this problem can be developed based on the 
[CoreOS Layering](https://github.com/coreos/enhancements/blob/main/os/coreos-layering.md) concepts, which proposes a 
new way of updating the underlying Operating System (OS) from OCI-compliant container images.

This tool aims at creating such OCI images plus bundling the main cluster artifacts and configurations in order to 
provide seed images that can be used during an image-based upgrade procedure that would drastically reduce the 
upgrading and reconfiguration times.  

### What does this tool do?

The purpose of the `ibu-imager` tool is to assist in the creation of IBU seed images, which are used later on by 
other components (e.g., [lifecycle-agent](https://github.com/openshift-kni/lifecycle-agent)) during an image-based 
upgrade procedure.

In that direction, the tool does the following: 

- Saves a list of container images used by `crio` (needed for pre-caching operations afterward)
- Creates a backup of the main platform configurations (e.g., `/var` and `/etc` directories, ostree artifacts, etc.)
- Encapsulates OCI images (as of now three: `backup`, `base`, and `parent`) and push them to a local registry (used 
during the image-based upgrade workflow afterward)

### Building

Building the binary locally.

```shell
-> make build 
go mod tidy && go mod vendor
Running go fmt
go fmt ./...
Running go vet
go vet ./...
go build -o bin/ibu-imager main.go
```

> **Note:** The binary can be found in `./bin/ibu-imager`.

Building and pushing the tool as container image.

```shell
-> make docker-build docker-push 
podman build -t jumphost.inbound.vz.bos2.lab:8443/lochoa/ibu-imager:4.14.0 -f Dockerfile .
[1/2] STEP 1/7: FROM registry.hub.docker.com/library/golang:1.19 AS builder
[1/2] STEP 2/7: WORKDIR /workspace
--> Using cache da22d00e2d1c5aeac0286448662b44b31c9e8d46ac6bf14e003b72df0342dd70
--> da22d00e2d1c
[1/2] STEP 3/7: COPY go.mod go.sum ./
--> Using cache 0f9d5f76a7c3a3aec156f680a6afa9bc0c7bae076c739dd7aed907a25274e1ea
--> 0f9d5f76a7c3
[1/2] STEP 4/7: COPY vendor/ vendor/
--> Using cache 9e78223560f0413aa88d6c6c4e2eb9bec3ac7f19bb9d422eb55847881cf8e828
--> 9e78223560f0
[1/2] STEP 5/7: COPY main.go main.go
--> Using cache 13197fedfb672adaec1fdcaf8cd09fa0f17fdd8aee399bef17ce8e883f8fc3d4
--> 13197fedfb67
[1/2] STEP 6/7: COPY cmd/ cmd/
--> 2e2b016d3591
[1/2] STEP 7/7: RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -mod=vendor -a -o ibu-imager main.go
--> a9fb405efff5
[2/2] STEP 1/4: FROM registry.ci.openshift.org/ocp/4.13:tools
[2/2] STEP 2/4: WORKDIR /
--> Using cache 5ab310acbf191f4c04e0e08ade233b2f1df75e50a58e27babb83c9b60666f578
--> 5ab310acbf19
[2/2] STEP 3/4: COPY --from=builder /workspace/ibu-imager .
--> cb1915277402
[2/2] STEP 4/4: ENTRYPOINT ["./ibu-imager"]
[2/2] COMMIT jumphost.inbound.vz.bos2.lab:8443/lochoa/ibu-imager:4.14.0
--> a60a10028253
Successfully tagged jumphost.inbound.vz.bos2.lab:8443/lochoa/ibu-imager:4.14.0
a60a10028253ba2b835df2a436bc51062f816cb6e081b53f3ac016479746ed0f
podman push jumphost.inbound.vz.bos2.lab:8443/lochoa/ibu-imager:4.14.0 --tls-verify=false
Getting image source signatures
Copying blob 9c3da9d5a92d skipped: already exists  
Copying blob 89969c818b60 skipped: already exists  
Copying blob 2e9427b8c823 skipped: already exists  
Copying blob dc9cabeae816 skipped: already exists  
Copying blob 68a3cb1b5de1 skipped: already exists  
Copying blob d2da84f6e5b7 skipped: already exists  
Copying blob 63b91bb26cad done  
Copying blob 492e513dbd66 skipped: already exists  
Copying config a60a100282 done  
Writing manifest to image destination
```

### Running the tool's help

To see the tool's help on your local host, run the following command:

```shell
-> ./bin/ibu-imager -h

 ___ ____  _   _            ___                                 
|_ _| __ )| | | |          |_ _|_ __ ___   __ _  __ _  ___ _ __ 
 | ||  _ \| | | |   _____   | ||  _   _ \ / _  |/ _  |/ _ \ '__|
 | || |_) | |_| |  |_____|  | || | | | | | (_| | (_| |  __/ |
|___|____/ \___/           |___|_| |_| |_|\__,_|\__, |\___|_|
                                                |___/

 A tool to assist building OCI seed images for Image Based Upgrades (IBU)

Usage:
  ibu-imager [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  create      Create OCI image and push it to a container registry.
  help        Help about any command

Flags:
  -h, --help       help for ibu-imager
  -c, --no-color   Control colored output
  -v, --verbose    Display verbose logs

Use "ibu-imager [command] --help" for more information about a command.
```

### Running as a container

To create an IBU seed image out of your Single Node OpenShift (SNO), run the following command directly on the node:

```shell
-> podman run -v /var:/var -v /var/lib:/var/lib -v /var/run:/var/run -v /etc:/etc -v /run/systemd/journal/socket:/run/systemd/journal/socket -v /tmp:/tmp \
              --privileged --pid=host --rm --network=host ${LOCAL_REGISTRY}/lochoa/ibu-imager:4.14.0 create \
              --authfile ${PATH_TO_AUTH_FILE} --registry ${TARGET_REGISTRY_FOR_OCI_IMAGES}

 ___ ____  _   _            ___                                 
|_ _| __ )| | | |          |_ _|_ __ ___   __ _  __ _  ___ _ __ 
 | ||  _ \| | | |   _____   | ||  _   _ \ / _  |/ _  |/ _ \ '__|
 | || |_) | |_| |  |_____|  | || | | | | | (_| | (_| |  __/ |
|___|____/ \___/           |___|_| |_| |_|\__,_|\__, |\___|_|
                                                |___/

 A tool to assist building OCI seed images for Image Based Upgrades (IBU)
	
time="2023-09-22 10:18:58" level=info msg="OCI image creation has started"
time="2023-09-22 10:18:58" level=info msg="Saving list of running containers and clusterversion."
time="2023-09-22 10:18:58" level=info msg="Skipping list of containers and clusterversion already exists."
time="2023-09-22 10:18:58" level=info msg="Stop kubelet service"
time="2023-09-22 10:18:58" level=info msg="Stopping containers and CRI-O runtime."
time="2023-09-22 10:18:58" level=info msg="Skipping running containers and CRI-O engine already stopped."
time="2023-09-22 10:18:58" level=info msg="Create backup datadir"
time="2023-09-22 10:18:58" level=info msg="Skipping var backup as it already exists."
time="2023-09-22 10:18:58" level=info msg="Skipping etc backup as it already exists."
time="2023-09-22 10:18:58" level=info msg="Skipping rpm-ostree backup as it already exists."
time="2023-09-22 10:18:58" level=info msg="Skipping mco-currentconfig backup as it already exists."
time="2023-09-22 10:18:58" level=info msg="Encapsulate and push backup OCI image."
time="2023-09-22 10:19:05" level=info msg="Encapsulate and push base OCI image."
time="2023-09-22 10:21:45" level=info msg="Encapsulate and push parent OCI image."
time="2023-09-22 10:21:45" level=info msg="OCI image has a parent commit to be encapsulated."
time="2023-09-22 10:24:21" level=info msg="OCI image created successfully!"
```

> **Note:** For a disconnected environment, first mirror the `ibu-imager` container image to your local registry using 
> [skopeo](https://github.com/containers/skopeo) or a similar tool.

## TODO

<details>
  <summary>TODO List</summary>

- [ ] Refactor wrapped bash commands (e.g., rpm-ostree commands) with stable go-bindings and/or libraries
- [ ] Fix all code TODO comments

</details>
