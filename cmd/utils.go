/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	// Default OCI image tag
	backupTag = "oneimage"
	// Pull secret. Written by the machine-config-operator
	imageRegistryAuthFile = "/var/lib/kubelet/config.json"
	// sourceDir is the directory where the datadir is backed up
	sourceDir = "/var"
	// backupDir is the directory where the ostree backup will be
	backupDir = "/var/tmp/backup"
	// Default kubeconfigFile location
	kubeconfigFile = "/etc/kubernetes/static-pod-resources/kube-apiserver-certs/secrets/node-kubeconfigs/lb-ext.kubeconfig"
)

// containerFileContent is the Dockerfile content for the IBU seed image
const containerFileContent = `
FROM scratch
COPY . /
`

// check is a helper function to simply check for errors
func check(err error) {
	if err != nil {
		log.Errorf("An error occurred: %s", err.Error())
		os.Exit(1)
	}
}

// runInHostNamespace execute a command in the host environment via nsenter
// inspired from: https://github.com/openshift/assisted-installer/blob/master/src/ops/ops.go#L881-L907
func runInHostNamespace(command string, args ...string) ([]byte, error) {

	arguments := []string{
		// nsenter is used here to launch processes inside the container in a way that makes said processes feel
		// and behave as if they're running on the host directly rather than inside the container
		"nsenter",
		"--target", "1",
		// Entering the cgroup namespace is not required for podman on CoreOS (where the
		// agent typically runs), but it's needed on some Fedora versions and
		// some other systemd-based systems. Those systems are used to run dry-mode
		// agents for load testing. If this flag is not used, Podman will sometimes
		// have trouble creating a systemd cgroup slice for new containers.
		"--cgroup",
		// The mount namespace is required for podman to access the host's container
		// storage
		"--mount",
		// The ipc option ensures that the nsenter command enters the same IPC namespace
		// as the init process (PID 1) before executing any commands.
		// This allows the command to access and manipulate IPC resources within that
		// specific namespace, if needed.
		"--ipc",
		"--pid",
		"--",
		command,
	}

	arguments = append(arguments, args...)
	log.Debugf("Running command: " + strings.Join(arguments, " "))

	cmd := exec.Command("bash", "-c", strings.Join(arguments, " "))
	if verbose {
		cmd.Stderr = os.Stderr
	}

	rawOutput, err := cmd.Output()
	check(err)

	return rawOutput, nil
}

// readLineFromFile reads the first line from a file and returns it as a string.
// It opens the file, scans for the first line, and closes the file when done.
// If no lines are found or an error occurs, it returns an error.
func readLineFromFile(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		return scanner.Text(), nil
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", fmt.Errorf("no lines found in the file")
}
