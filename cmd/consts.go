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

const (
	// Default OCI image tag
	backupTag = "oneimage"
	// Pull secret. Written by the machine-config-operator
	imageRegistryAuthFile = "/var/lib/kubelet/config.json"
	// backupDir is the directory where the ostree backup will be
	backupDir = "/var/tmp/backup"
	// Default kubeconfigFile location
	kubeconfigFile = "/etc/kubernetes/static-pod-resources/kube-apiserver-certs/secrets/node-kubeconfigs/lb-ext.kubeconfig"
)
