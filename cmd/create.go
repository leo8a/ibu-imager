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

// rpmOstreeClient creates a new rpm ostree client for the IBU imager
var rpmOstreeClient = NewClient("ibu-imager")

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
	// Save list of running containers and current clusterversion
	//
	log.Println("Saving list of running containers, catalogsources, and clusterversion.")

	// Check if the file /var/tmp/container_list.done does not exist
	if _, err = os.Stat("/var/tmp/container_list.done"); os.IsNotExist(err) {

		// Create the directory /var/tmp/backup if it doesn't exist
		log.Debug("Create backup directory at " + backupDir)
		err = os.MkdirAll(backupDir, os.ModePerm)
		check(err)

		// Execute 'crictl images -o json' command, parse the JSON output and extract image references using 'jq'
		log.Debug("Save list of running containers")
		_, err = runInHostNamespace(
			"crictl", append([]string{"images", "-o", "json", "|", "jq", "-r", "'.images[] | .repoDigests[], .repoTags[]'"}, ">", backupDir+"/containers.list")...)
		check(err)

		// Execute 'oc get catalogsource' command, parse the JSON output and extract image references using 'jq'
		log.Debug("Save catalog source images")
		_, err = runInHostNamespace(
			"oc", append([]string{"get", "catalogsource", "-A", "-o", "json", "--kubeconfig", kubeconfigFile, "|", "jq", "-r", "'.items[].spec.image'"}, ">", backupDir+"/catalogimages.list")...)
		check(err)

		// Execute 'oc get clusterversion' command and save it
		log.Debug("Save clusterversion to file")
		_, err = runInHostNamespace(
			"oc", append([]string{"get", "clusterversion", "version", "-o", "json", "--kubeconfig", kubeconfigFile}, ">", backupDir+"/clusterversion.json")...)
		check(err)

		// Create the file /var/tmp/container_list.done
		_, err = os.Create("/var/tmp/container_list.done")
		check(err)

		log.Println("List of containers, catalogsources, and clusterversion saved successfully.")
	} else {
		log.Println("Skipping list of containers, catalogsources, and clusterversion already exists.")
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
	_, err = runInHostNamespace(
		"systemctl", append([]string{"is-active", "crio"}, ">", backupDir+"/crio.systemd.status")...)
	check(err)

	// Read CRI-O systemd status from file
	crioSystemdStatus, _ := readLineFromFile(backupDir + "/crio.systemd.status")

	if crioSystemdStatus == "active" {

		// CRI-O is active, so stop running containers
		log.Debug("Stop running containers")
		_, err = runInHostNamespace(
			"crictl", []string{"ps", "-q", "|", "xargs", "--no-run-if-empty", "--max-args", "1", "--max-procs", "10", "crictl", "stop", "--timeout", "5"}...)
		check(err)

		// Waiting for containers to stop (TODO: implement this using runInHostNamespace)
		//waitCMD := fmt.Sprintf(`while crictl ps -q | grep -q . ; do sleep 1 ; done`)
		//log.Debug("Wait for containers to stop")
		//err = runCMD(waitCMD)
		//check(err)

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
	varTarFile := backupDir + "/var.tgz"
	if _, err = os.Stat(varTarFile); os.IsNotExist(err) {

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
		tarArgs := []string{"czf", varTarFile}
		for _, pattern := range excludePatterns {
			// We're handling the excluded patterns in bash, we need to single quote them to prevent expansion
			tarArgs = append(tarArgs, "--exclude", fmt.Sprintf("'%s'", pattern))
		}
		tarArgs = append(tarArgs, "--selinux", sourceDir)

		// Run the tar command
		_, err = runInHostNamespace("tar", strings.Join(tarArgs, " "))
		check(err)

		log.Println("Backup of /var created successfully.")
	} else {
		log.Println("Skipping var backup as it already exists.")
	}

	// Check if the backup file for /etc doesn't exist
	if _, err = os.Stat(backupDir + "/etc.tgz"); os.IsNotExist(err) {

		// Execute 'ostree admin config-diff' command and backup /etc
		_, err = runInHostNamespace(
			"ostree", []string{"admin", "config-diff", "|", "awk", `'{print "/etc/" $2}'`, "|", "xargs", "tar", "czf", backupDir + "/etc.tgz", "--selinux"}...)
		check(err)

		log.Println("Backup of /etc created successfully.")
	} else {
		log.Println("Skipping etc backup as it already exists.")
	}

	// Check if the backup file for ostree doesn't exist
	if _, err = os.Stat(backupDir + "/ostree.tgz"); os.IsNotExist(err) {

		// Execute 'tar' command and backup /etc
		_, err = runInHostNamespace(
			"tar", []string{"czf", backupDir + "/ostree.tgz", "--selinux", "-C", "/ostree/repo", "."}...)
		check(err)

		log.Println("Backup of ostree created successfully.")
	} else {
		log.Println("Skipping ostree backup as it already exists.")
	}

	// Check if the commit backup doesn't exist
	if _, err = os.Stat(backupDir + "/ostree.commit"); os.IsNotExist(err) {

		// Execute 'ostree commit' command
		_, err = runInHostNamespace(
			"ostree", append([]string{"commit", "--branch", backupTag, backupDir}, ">", backupDir+"/ostree.commit")...)
		check(err)

		log.Debug("Commit backup created successfully.")
	} else {
		log.Debug("Skipping backup commit as it already exists.")
	}

	//
	// Building and pushing OCI image
	//
	log.Printf("Build and push OCI image to %s:%s.", containerRegistry, backupTag)
	log.Debug(rpmOstreeClient.RpmOstreeVersion()) // If verbose, also dump out current rpm-ostree version available

	// Get the current status of rpm-ostree daemon in the host
	statusRpmOstree, err := rpmOstreeClient.QueryStatus()
	check(err)

	// Get OSName for booted ostree deployment
	bootedOSName := statusRpmOstree.Deployments[0].OSName

	// Get ID for booted ostree deployment
	bootedID := statusRpmOstree.Deployments[0].ID

	// Get SHA for booted ostree deployment
	bootedDeployment := strings.Split(bootedID, "-")[1]

	// Check if the backup file for .origin doesn't exist
	originFileName := fmt.Sprintf("%s/ostree-%s.origin", backupDir, bootedDeployment)
	if _, err = os.Stat(originFileName); os.IsNotExist(err) {

		// Execute 'copy' command and backup .origin file
		_, err = runInHostNamespace(
			"cp", []string{"/ostree/deploy/" + bootedOSName + "/deploy/" + bootedDeployment + ".origin", originFileName}...)
		check(err)

		log.Println("Backup of .origin created successfully.")
	} else {
		log.Println("Skipping .origin backup as it already exists.")
	}

	// Create a temporary file for the Dockerfile content
	tmpfile, err := os.CreateTemp("/var/tmp", "dockerfile-")
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
	_, err = runInHostNamespace(
		"podman", []string{"build",
			"-f", tmpfile.Name(),
			"-t", containerRegistry + ":" + backupTag,
			"--build-context", "ostreerepo=/sysroot/ostree/repo",
			backupDir}...)
	check(err)

	// Push the created OCI image to user's repository
	_, err = runInHostNamespace(
		"podman", []string{"push",
			"--authfile", authFile,
			containerRegistry + ":" + backupTag}...)
	check(err)

	log.Printf("OCI image created successfully!")
}
