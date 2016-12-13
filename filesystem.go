//
// Copyright (c) 2016 Intel Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package virtcontainers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	//	"github.com/01org/ciao/ssntp/uuid"
	//	"github.com/golang/glog"
)

// podResource is an int representing a pod resource type
type podResource int

const (
	// configFileType represents a configuration file type
	configFileType podResource = iota

	// stateFileType represents a state file type
	stateFileType

	// networkFileType represents a network file type
	networkFileType

	// lockFileType represents a lock file type
	lockFileType
)

// configStoragePath is the pod configuration directory.
// It will contain one config.json file for each created pod.
const configStoragePath = "/var/lib/virtcontainers/pods"

// runStoragePath is the pod runtime directory.
// It will contain one state.json and one lock file for each created pod.
const runStoragePath = "/run/virtcontainers/pods"

// configFile is the file name used for every JSON pod configuration.
const configFile = "config.json"

// stateFile is the file name storing a pod state.
const stateFile = "state.json"

// networkFile is the file name storing a pod network.
const networkFile = "network.json"

// lockFile is the file name locking the usage of a pod.
const lockFileName = "lock"

// resourceStorage is the virtcontainers resources (configuration, state, etc...)
// storage interface.
// The default resource storage implementation is filesystem.
type resourceStorage interface {
	// Create all resources for a pod
	createAllResources(pod Pod) error

	// Resources URIs functions return both the URI
	// for the actual resource and the URI base.
	containerURI(podID, containerID string, resource podResource) (string, string, error)
	podURI(podID string, resource podResource) (string, string, error)

	// Pod resources
	storePodResource(podID string, resource podResource, data interface{}) error
	deletePodResources(podID string, resources []podResource) error
	fetchPodConfig(podID string) (PodConfig, error)
	fetchPodState(podID string) (State, error)
	fetchPodNetwork(podID string) ([]NetworkInterfacePair, error)

	// Container resources
	storeContainerResource(podID, containerID string, resource podResource, data interface{}) error
	deleteContainerResources(podID, containerID string, resources []podResource) error
	fetchContainerConfig(podID, containerID string) (ContainerConfig, error)
	fetchContainerState(podID, containerID string) (State, error)
}

// filesystem is a resourceStorage interface implementation for a local filesystem.
type filesystem struct {
}

func (fs *filesystem) createAllResources(pod Pod) error {
	_, path, _ := fs.podURI(pod.id, stateFileType)
	err := os.MkdirAll(path, os.ModeDir)
	if err != nil {
		return err
	}

	_, path, _ = fs.podURI(pod.id, configFileType)
	err = os.MkdirAll(path, os.ModeDir)
	if err != nil {
		return err
	}

	for _, container := range pod.containers {
		_, path, _ = fs.containerURI(pod.id, container.ID, configFileType)
		err = os.MkdirAll(path, os.ModeDir)
		if err != nil {
			fs.deletePodResources(pod.id, nil)
			return err
		}

		_, path, _ = fs.containerURI(pod.id, container.ID, stateFileType)
		err = os.MkdirAll(path, os.ModeDir)
		if err != nil {
			fs.deletePodResources(pod.id, nil)
			return err
		}
	}

	podlockFile, _, err := fs.podURI(pod.id, lockFileType)
	if err != nil {
		fs.deletePodResources(pod.id, nil)
		return err
	}

	_, err = os.Stat(podlockFile)
	if err != nil {
		lockFile, err := os.Create(podlockFile)
		if err != nil {
			fs.deletePodResources(pod.id, nil)
			return err
		}
		lockFile.Close()
	}

	return nil
}

func (fs *filesystem) storeFile(path string, data interface{}) error {
	_, err := os.Stat(path)
	if err == nil {
		os.Remove(path)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	jsonOut, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("Could not marshall data: %s", err)
	}
	f.Write(jsonOut)

	return nil
}

func (fs *filesystem) fetchFile(path string, data interface{}) error {
	_, err := os.Stat(path)
	if err != nil {
		return err
	}

	fileData, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	err = json.Unmarshal([]byte(string(fileData)), data)
	if err != nil {
		return err
	}

	return nil
}

func resourceDir(podID, containerID string, resource podResource) (string, error) {
	var path string

	if podID == "" {
		return "", fmt.Errorf("PodID cannot be empty")
	}

	switch resource {
	case configFileType:
		path = configStoragePath
		break
	case stateFileType, networkFileType, lockFileType:
		path = runStoragePath
		break
	default:
		return "", fmt.Errorf("Invalid pod resource")
	}

	dirPath := filepath.Join(path, podID, containerID)

	return dirPath, nil
}

func (fs *filesystem) resourceURI(podID, containerID string, resource podResource) (string, string, error) {
	var filename string

	if podID == "" {
		return "", "", fmt.Errorf("Pod ID cannot be empty")
	}

	dirPath, err := resourceDir(podID, containerID, resource)
	if err != nil {
		return "", "", err
	}

	switch resource {
	case configFileType:
		filename = configFile
		break
	case stateFileType:
		filename = stateFile
	case networkFileType:
		filename = networkFile
	case lockFileType:
		filename = lockFileName
		break
	default:
		return "", "", fmt.Errorf("Invalid pod resource")
	}

	filePath := filepath.Join(dirPath, filename)

	return filePath, dirPath, nil
}

func (fs *filesystem) containerURI(podID, containerID string, resource podResource) (string, string, error) {
	if containerID == "" {
		return "", "", fmt.Errorf("Container ID cannot be empty")
	}

	return fs.resourceURI(podID, containerID, resource)
}

func (fs *filesystem) podURI(podID string, resource podResource) (string, string, error) {
	return fs.resourceURI(podID, "", resource)
}

func (fs *filesystem) storeResource(podID, containerID string, resource podResource, data interface{}) error {
	switch file := data.(type) {
	case PodConfig, ContainerConfig:
		if resource != configFileType {
			return fmt.Errorf("Invalid pod resource")
		}

		configFile, _, err := fs.resourceURI(podID, containerID, configFileType)
		if err != nil {
			return err
		}

		return fs.storeFile(configFile, file)

	case State:
		if resource != stateFileType {
			return fmt.Errorf("Invalid pod resource")
		}

		stateFile, _, err := fs.resourceURI(podID, containerID, stateFileType)
		if err != nil {
			return err
		}

		return fs.storeFile(stateFile, file)

	case NetworkInterfacePairs:
		if resource != networkFileType {
			return fmt.Errorf("Invalid pod resource")
		}

		networkFile, _, err := fs.resourceURI(podID, containerID, networkFileType)
		if err != nil {
			return err
		}

		return fs.storeFile(networkFile, file)

	default:
		return fmt.Errorf("Invalid resource data type")
	}
}

func (fs *filesystem) fetchResource(podID, containerID string, resource podResource) (interface{}, error) {
	path, _, err := fs.resourceURI(podID, containerID, resource)
	if err != nil {
		return nil, err
	}

	switch resource {
	case configFileType:
		if containerID == "" {
			config := PodConfig{}
			err = fs.fetchFile(path, &config)
			if err != nil {
				return nil, err
			}

			return config, nil
		}

		config := ContainerConfig{}
		err = fs.fetchFile(path, &config)
		if err != nil {
			return nil, err
		}

		return config, nil

	case stateFileType:
		state := State{}
		err = fs.fetchFile(path, &state)
		if err != nil {
			return nil, err
		}

		return state, nil

	case networkFileType:
		netPairs := []NetworkInterfacePair{}
		err = fs.fetchFile(path, &netPairs)
		if err != nil {
			return nil, err
		}

		return NetworkInterfacePairs(netPairs), nil
	}

	return nil, fmt.Errorf("Invalid pod resource")
}

func (fs *filesystem) storePodResource(podID string, resource podResource, data interface{}) error {
	return fs.storeResource(podID, "", resource, data)
}

func (fs *filesystem) fetchPodConfig(podID string) (PodConfig, error) {
	data, err := fs.fetchResource(podID, "", configFileType)
	if err != nil {
		return PodConfig{}, err
	}

	switch config := data.(type) {
	case PodConfig:
		return config, nil
	}

	return PodConfig{}, fmt.Errorf("Unknown config type")
}

func (fs *filesystem) fetchPodState(podID string) (State, error) {
	data, err := fs.fetchResource(podID, "", stateFileType)
	if err != nil {
		return State{}, err
	}

	switch state := data.(type) {
	case State:
		return state, nil
	}

	return State{}, fmt.Errorf("Unknown state type")
}

func (fs *filesystem) fetchPodNetwork(podID string) ([]NetworkInterfacePair, error) {
	data, err := fs.fetchResource(podID, "", networkFileType)
	if err != nil {
		return []NetworkInterfacePair{}, err
	}

	switch netPairs := data.(type) {
	case NetworkInterfacePairs:
		return netPairs, nil
	}

	return []NetworkInterfacePair{}, fmt.Errorf("Unknown network type")
}

func (fs *filesystem) deletePodResources(podID string, resources []podResource) error {
	if resources == nil {
		resources = []podResource{configFileType, stateFileType}
	}

	for _, resource := range resources {
		_, dir, err := fs.podURI(podID, resource)
		if err != nil {
			return err
		}

		err = os.RemoveAll(dir)
		if err != nil {
			return err
		}
	}

	return nil
}

func (fs *filesystem) storeContainerResource(podID, containerID string, resource podResource, data interface{}) error {
	if containerID == "" {
		return fmt.Errorf("Container ID cannot be empty")
	}

	return fs.storeResource(podID, containerID, resource, data)
}

func (fs *filesystem) fetchContainerConfig(podID, containerID string) (ContainerConfig, error) {
	if containerID == "" {
		return ContainerConfig{}, fmt.Errorf("Container ID cannot be empty")
	}

	data, err := fs.fetchResource(podID, containerID, configFileType)
	if err != nil {
		return ContainerConfig{}, err
	}

	switch config := data.(type) {
	case ContainerConfig:
		return config, nil
	}

	return ContainerConfig{}, fmt.Errorf("Unknown config type")
}

func (fs *filesystem) fetchContainerState(podID, containerID string) (State, error) {
	if containerID == "" {
		return State{}, fmt.Errorf("Container ID cannot be empty")
	}

	data, err := fs.fetchResource(podID, containerID, stateFileType)
	if err != nil {
		return State{}, err
	}

	switch state := data.(type) {
	case State:
		return state, nil
	}

	return State{}, fmt.Errorf("Unknown state type")
}

func (fs *filesystem) deleteContainerResources(podID, containerID string, resources []podResource) error {
	if resources == nil {
		resources = []podResource{configFileType, stateFileType}
	}

	for _, resource := range resources {
		_, dir, err := fs.podURI(podID, resource)
		if err != nil {
			return err
		}

		containerDir := filepath.Join(dir, containerID, "/")

		err = os.RemoveAll(containerDir)
		if err != nil {
			return err
		}
	}

	return nil
}
