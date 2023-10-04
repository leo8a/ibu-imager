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
		err = runCMD("touch /var/tmp/container_list.done")
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

		// Waiting for containers to stop (repeated)
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
		err = runCMD("tar" + " " + strings.Join(args, " "))
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
	if _, err := os.Stat(backupDir + "/mco-currentconfig.json"); os.IsNotExist(err) {

		// Execute 'ostree admin config-diff' command and backup mco-currentconfig
		backupCurrentConfigCMD := fmt.Sprintf(`cp /etc/machine-config-daemon/currentconfig %s/mco-currentconfig.json`, backupDir)
		err = runCMD(backupCurrentConfigCMD)
		check(err)

		log.Println("Backup of mco-currentconfig created successfully.")
	} else {
		log.Println("Skipping mco-currentconfig backup as it already exists.")
	}

	// Check if the commit backup doesn't exist
	if _, err := os.Stat(backupDir + "/ostree.commit"); os.IsNotExist(err) {

		// Execute 'ostree commit' command
		ostreeCommitCMD := fmt.Sprintf(`nsenter --target 1 --cgroup --mount --ipc --pid -- ostree commit --branch %s %s > %s/ostree.commit`, backupTag, backupDir, backupDir)
		err = runCMD(ostreeCommitCMD)
		check(err)

		log.Debug("Commit backup created successfully.")
	} else {
		log.Debug("Skipping backup commit as it already exists.")
	}

	//
	// Encapsulating and pushing backup OCI image
	//
	log.Printf("Encapsulate and push backup OCI image to %s:%s.", containerRegistry, backupTag)

	// Execute 'ostree container encapsulate' command for backup OCI image
	ostreeEncapsulateBackupCMD := fmt.Sprintf(`nsenter --target 1 --cgroup --mount --ipc --pid sh -c 'REGISTRY_AUTH_FILE=%s ostree container encapsulate %s registry:%s:%s --repo /ostree/repo --label ostree.bootable=true'`, authFile, backupTag, containerRegistry, backupTag)
	err = runCMD(ostreeEncapsulateBackupCMD)
	check(err)

	//
	// Encapsulating and pushing base OCI image
	//
	log.Printf("Encapsulate and push base OCI image to %s:%s.", containerRegistry, baseTag)

	// Create base commit checksum file
	ostreeBaseChecksumCMD := fmt.Sprintf(`nsenter --target 1 --cgroup --mount --ipc --pid -- rpm-ostree status -v --json | jq -r '.deployments[] | select(.booted == true).checksum' > /var/tmp/ostree.base.commit`)
	err = runCMD(ostreeBaseChecksumCMD)
	check(err)

	// Read base commit from file
	baseCommit, err := readLineFromFile("/var/tmp/ostree.base.commit")

	// Execute 'ostree container encapsulate' command for base OCI image
	ostreeEncapsulateBaseCMD := fmt.Sprintf(`nsenter --target 1 --cgroup --mount --ipc --pid sh -c 'REGISTRY_AUTH_FILE=%s ostree container encapsulate %s registry:%s:%s --repo /ostree/repo --label ostree.bootable=true'`, authFile, baseCommit, containerRegistry, baseTag)
	err = runCMD(ostreeEncapsulateBaseCMD)
	check(err)

	//
	// Encapsulating and pushing parent OCI image
	//

	// Create parent checksum file
	ostreeHasParentChecksumCMD := fmt.Sprintf(`nsenter --target 1 --cgroup --mount --ipc --pid -- rpm-ostree status -v --json | jq -r '.deployments[] | select(.booted == true) | has("base-checksum")' > /var/tmp/ostree.has.parent`)
	err = runCMD(ostreeHasParentChecksumCMD)
	check(err)

	// Read hasParent commit from file
	hasParent, err := readLineFromFile("/var/tmp/ostree.has.parent")

	// Check if current ostree deployment has a parent commit
	if hasParent == "true" {
		log.Info("OCI image has a parent commit to be encapsulated.")

		// Create parent commit checksum file
		ostreeParentChecksumCMD := fmt.Sprintf(`nsenter --target 1 --cgroup --mount --ipc --pid -- rpm-ostree status -v --json | jq -r '.deployments[] | select(.booted == true)."base-checksum"' > /var/tmp/ostree.parent.commit`)
		err = runCMD(ostreeParentChecksumCMD)
		check(err)

		// Read parent commit from file
		parentCommit, err := readLineFromFile("/var/tmp/ostree.parent.commit")

		// Execute 'ostree container encapsulate' command for parent OCI image
		log.Printf("Encapsulate and push parent OCI image to %s:%s.", containerRegistry, parentTag)
		ostreeEncapsulateParentCMD := fmt.Sprintf(`nsenter --target 1 --cgroup --mount --ipc --pid sh -c 'REGISTRY_AUTH_FILE=%s ostree container encapsulate %s registry:%s:%s --repo /ostree/repo --label ostree.bootable=true'`, authFile, parentCommit, containerRegistry, parentTag)
		err = runCMD(ostreeEncapsulateParentCMD)
		check(err)

	} else {
		log.Info("Skipping encapsulate parent commit as it is not present.")
	}

	log.Printf("OCI image created successfully!")
}
