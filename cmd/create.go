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
	"fmt"
	"os"
	"strings"

	"github.com/godbus/dbus"
	"github.com/spf13/cobra"
	"io/ioutil"
)

// authFile is the path to the registry credentials used to push the OCI image
var authFile string

// containerRegistry is the registry to push the OCI image
var containerRegistry string

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create OCI image and push it to a container registry.",
	Run: func(cmd *cobra.Command, args []string) {
		create()
	},
}

func init() {

	// Add create command
	rootCmd.AddCommand(createCmd)

	// Add flags related to container registry
	createCmd.Flags().StringVarP(&authFile, "authfile", "a", imageRegistryAuthFile, "The path to the authentication file of the container registry.")
	createCmd.Flags().StringVarP(&containerRegistry, "registry", "r", "", "The container registry used to push the OCI image.")
}

func create() {

	var err error
	log.Printf("OCI image creation has started")

	// Check if containerRegistry was provided by the user
	if containerRegistry == "" {
		fmt.Printf(" *** Please provide a valid container registry to store the created OCI images *** \n")
		log.Info("Skipping OCI image creation.")
		return
	}

	// Connect to the system bus
	conn, err := dbus.SystemBus()
	if err != nil {
		log.Errorf("Failed to connect to D-Bus: %v", err)
	}

	// Create systemdObj to represent the systemd D-Bus interface
	// used to stop kubelet and crio systemd services later on
	systemdObj := conn.Object("org.freedesktop.systemd1", "/org/freedesktop/systemd1")

	//
	// Save list of running containers
	//
	log.Println("Saving list of running containers.")

	// Check if the file /var/tmp/container_list.done does not exist
	if _, err = os.Stat("/var/tmp/container_list.done"); os.IsNotExist(err) {

		// Create the directory /var/tmp/backup if it doesn't exist
		log.Debug("Create backup directory at " + backupDir)
		err = os.MkdirAll(backupDir, os.ModePerm)
		check(err)

		// Execute 'crictl ps -o json' command, parse the JSON output and extract image references using 'jq'
		log.Debug("Save list of running containers")
		criListContainers := fmt.Sprintf(`nsenter --target 1 --cgroup --mount --ipc --pid -- crictl images -o json | jq -r '.images[] | .repoDigests[], .repoTags[]' > ` + backupDir + `/containers.list`)
		err = runCMD(criListContainers)
		check(err)

		// Create the file /var/tmp/container_list.done
		_, err = os.Create("/var/tmp/container_list.done")
		check(err)

		log.Println("List of containers saved successfully.")
	} else {
		log.Println("Skipping list of containers already exists.")
	}

	//
	// Stop kubelet service
	//
	log.Println("Stop kubelet service")

	// Execute a D-Bus call to stop the kubelet service
	err = systemdObj.Call("org.freedesktop.systemd1.Manager.StopUnit", 0, "kubelet.service", "replace").Err
	check(err)

	//
	// Stopping containers and CRI-O runtime
	//
	log.Println("Stopping containers and CRI-O runtime.")

	// Store current status of CRI-O systemd
	crioService := fmt.Sprintf(`nsenter --target 1 --cgroup --mount --ipc --pid -- systemctl is-active crio > %s/crio.systemd.status`, backupDir)
	_ = runCMD(crioService) // this commands returns 3 when crio is inactive

	// Read CRI-O systemd status from file
	crioSystemdStatus, _ := readLineFromFile(backupDir + "/crio.systemd.status")

	if crioSystemdStatus == "active" {

		// CRI-O is active, so stop running containers
		criStopContainers := fmt.Sprintf(`crictl ps -q | xargs --no-run-if-empty --max-args 1 --max-procs 10 crictl stop --timeout 5`)
		log.Debug("Stop running containers")
		err = runCMD(criStopContainers)
		check(err)

		// Waiting for containers to stop
		waitCMD := fmt.Sprintf(`while crictl ps -q | grep -q . ; do sleep 1 ; done`)
		log.Debug("Wait for containers to stop")
		err = runCMD(waitCMD)
		check(err)

		// Execute a D-Bus call to stop the CRI-O runtime
		log.Debug("Stopping CRI-O engine")
		err = systemdObj.Call("org.freedesktop.systemd1.Manager.StopUnit", 0, "crio.service", "replace").Err
		check(err)

		log.Println("Running containers and CRI-O engine stopped successfully.")
	} else {
		log.Println("Skipping running containers and CRI-O engine already stopped.")
	}

	//
	// Create backup datadir
	//
	log.Println("Create backup datadir")

	// Check if the backup file for /var doesn't exist
	if _, err := os.Stat(backupDir + "/var.tgz"); os.IsNotExist(err) {

		// Define the 'exclude' patterns
		excludePatterns := []string{
			"/var/tmp/*",
			"/var/lib/log/*",
			"/var/log/*",
			"/var/lib/containers/*",
			"/var/lib/kubelet/pods/*",
			"/var/lib/cni/bin/*",
		}

		// Build the tar command
		args := []string{"czf", fmt.Sprintf("%s/var.tgz", backupDir)}
		for _, pattern := range excludePatterns {
			// We're handling the excluded patterns in bash, we need to single quote them to prevent expansion
			args = append(args, "--exclude", fmt.Sprintf("'%s'", pattern))
		}
		args = append(args, "--selinux", sourceDir)

		// Run the tar command
		err = runCMD("tar " + strings.Join(args, " "))
		check(err)

		log.Println("Backup of /var created successfully.")
	} else {
		log.Println("Skipping var backup as it already exists.")
	}

	// Check if the backup file for /etc doesn't exist
	if _, err := os.Stat(backupDir + "/etc.tgz"); os.IsNotExist(err) {

		// Execute 'ostree admin config-diff' command and backup /etc
		ostreeAdminCMD := fmt.Sprintf(`nsenter --target 1 --cgroup --mount --ipc --pid -- ostree admin config-diff | awk '{print "/etc/" $2}' | xargs tar czf %s/etc.tgz --selinux`, backupDir)
		err = runCMD(ostreeAdminCMD)
		check(err)

		log.Println("Backup of /etc created successfully.")
	} else {
		log.Println("Skipping etc backup as it already exists.")
	}

	// Check if the backup file for rpm-ostree doesn't exist
	if _, err := os.Stat(backupDir + "/rpm-ostree.json"); os.IsNotExist(err) {

		// Execute 'rpm-ostree status' command and backup its output
		rpmOStreeCMD := fmt.Sprintf(`nsenter --target 1 --cgroup --mount --ipc --pid -- rpm-ostree status -v --json > %s/rpm-ostree.json`, backupDir)
		err = runCMD(rpmOStreeCMD)
		check(err)

		log.Println("Backup of rpm-ostree created successfully.")
	} else {
		log.Println("Skipping rpm-ostree backup as it already exists.")
	}

	// Check if the backup file for mco-currentconfig doesn't exist
	if _, err = os.Stat(backupDir + "/mco-currentconfig.json"); os.IsNotExist(err) {

		// Execute 'copy' command and backup mco-currentconfig
		backupCurrentConfigCMD := fmt.Sprintf(`cp /etc/machine-config-daemon/currentconfig %s/mco-currentconfig.json`, backupDir)
		err = runCMD(backupCurrentConfigCMD)
		check(err)

		log.Println("Backup of mco-currentconfig created successfully.")
	} else {
		log.Println("Skipping mco-currentconfig backup as it already exists.")
	}

	// Check if the commit backup doesn't exist
	if _, err = os.Stat(backupDir + "/ostree.commit"); os.IsNotExist(err) {

		// Execute 'ostree commit' command
		ostreeCommitCMD := fmt.Sprintf(`nsenter --target 1 --cgroup --mount --ipc --pid -- ostree commit --branch %s %s > %s/ostree.commit`, backupTag, backupDir, backupDir)
		err = runCMD(ostreeCommitCMD)
		check(err)

		log.Debug("Commit backup created successfully.")
	} else {
		log.Debug("Skipping backup commit as it already exists.")
	}

	//
	// Building and pushing OCI image
	//
	log.Printf("Build and push OCI image to %s:%s.", containerRegistry, backupTag)

	// Get the current ostree deployment name booted and save it
	bootedOSName := fmt.Sprintf(
		`nsenter --target 1 --cgroup --mount --ipc --pid -- rpm-ostree status -v --json | jq -r '.deployments[] | select(.booted == true) | .osname' > /var/tmp/booted.osname`)
	err = runCMD(bootedOSName)
	check(err)

	// Get the current ostree deployment id booted and save it
	bootedID := fmt.Sprintf(
		`nsenter --target 1 --cgroup --mount --ipc --pid -- rpm-ostree status -v --json | jq -r '.deployments[] | select(.booted == true) | .id' > /var/tmp/booted.id`)
	err = runCMD(bootedID)
	check(err)

	// Read current ostree deployment name from file
	bootedOSName_, err := readLineFromFile("/var/tmp/booted.osname")
	check(err)

	// Read current ostree deployment id from file
	bootedID_, err := readLineFromFile("/var/tmp/booted.id")
	check(err)

	// Get booted ostree deployment sha
	bootedDeployment := strings.Split(bootedID_, "-")[1]

	// Check if the backup file for .origin doesn't exist
	originFileName := fmt.Sprintf("%s/ostree-%s.origin", backupDir, bootedDeployment)
	if _, err := os.Stat(originFileName); os.IsNotExist(err) {

		// Execute 'copy' command and backup mco-currentconfig
		backupOriginCMD := fmt.Sprintf(
			`nsenter --target 1 --cgroup --mount --ipc --pid -- cp /ostree/deploy/%s/deploy/%s.origin %s`, bootedOSName_, bootedDeployment, originFileName)
		err = runCMD(backupOriginCMD)
		check(err)

		log.Println("Backup of .origin created successfully.")
	} else {
		log.Println("Skipping .origin backup as it already exists.")
	}

	// Create a temporary file for the Dockerfile content
	tmpfile, err := ioutil.TempFile("/var/tmp", "dockerfile-")
	if err != nil {
		log.Errorf("Error creating temporary file: %s", err)
	}
	defer os.Remove(tmpfile.Name()) // Clean up the temporary file

	// Write the content to the temporary file
	_, err = tmpfile.WriteString(containerFileContent)
	if err != nil {
		log.Errorf("Error writing to temporary file: %s", err)
	}
	tmpfile.Close() // Close the temporary file

	// Build the single OCI image (note: We could include --squash-all option, as well)
	containerBuildCMD := fmt.Sprintf(
		`nsenter --target 1 --cgroup --mount --ipc --pid -- podman build -f %s -t %s:%s --build-context ostreerepo=/sysroot/ostree/repo %s`,
		tmpfile.Name(), containerRegistry, backupTag, backupDir)
	err = runCMD(containerBuildCMD)
	check(err)

	// Push the created OCI image to user's repository
	containerPushCMD := fmt.Sprintf(
		`nsenter --target 1 --cgroup --mount --ipc --pid -- podman push --authfile %s %s:%s`,
		authFile, containerRegistry, backupTag)
	err = runCMD(containerPushCMD)
	check(err)

	log.Printf("OCI image created successfully!")
}
