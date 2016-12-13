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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/01org/ciao/ssntp/uuid"
	"github.com/golang/glog"
)

// controlSocket is the pod control socket.
// It is an hypervisor resource, and for example qemu's control
// socket is the QMP one.
const controlSocket = "ctrl.sock"

// monitorSocket is the pod monitoring socket.
// It is an hypervisor resource, and is a qmp socket in the qemu case.
// This is a socket that any monitoring entity will listen to in order
// to understand if the VM is still alive or not.
const monitorSocket = "monitor.sock"

// stateString is a string representing a pod state.
type stateString string

const (
	// stateReady represents a pod/container that's ready to be run
	stateReady stateString = "ready"

	// stateRunning represents a pod/container that's currently running.
	stateRunning = "running"

	// statePaused represents a pod/container that has been paused.
	statePaused = "paused"
)

// State is a pod state structure.
type State struct {
	State stateString `json:"state"`
}

// valid checks that the pod state is valid.
func (state *State) valid() bool {
	for _, validState := range []stateString{stateReady, stateRunning, statePaused} {
		if state.State == validState {
			return true
		}
	}

	return false
}

// validTransition returns an error if we want to move to
// an unreachable state.
func (state *State) validTransition(oldState stateString, newState stateString) error {
	if state.State != oldState {
		return fmt.Errorf("Invalid state %s (Expecting %s)", state.State, oldState)
	}

	switch state.State {
	case stateReady:
		if newState == stateRunning {
			return nil
		}

	case stateRunning:
		if newState == statePaused || newState == stateReady {
			return nil
		}

	case statePaused:
		if newState == stateRunning {
			return nil
		}
	}

	return fmt.Errorf("Can not move from %s to %s",
		state.State, newState)
}

// Volume is a shared volume between the host and the VM,
// defined by its mount tag and its host path.
type Volume struct {
	// MountTag is a label used as a hint to the guest.
	MountTag string

	// HostPath is the host filesystem path for this volume.
	HostPath string
}

// Volumes is a Volume list.
type Volumes []Volume

// Set assigns volume values from string to a Volume.
func (v *Volumes) Set(volStr string) error {
	volSlice := strings.Split(volStr, " ")

	for _, vol := range volSlice {
		volArgs := strings.Split(vol, ":")

		if len(volArgs) != 2 {
			return fmt.Errorf("Wrong string format: %s, expecting only 2 parameters separated with ':'", vol)
		}

		if volArgs[0] == "" || volArgs[1] == "" {
			return fmt.Errorf("Volume parameters cannot be empty")
		}

		volume := Volume{
			MountTag: volArgs[0],
			HostPath: volArgs[1],
		}

		*v = append(*v, volume)
	}

	return nil
}

// String converts a Volume to a string.
func (v *Volumes) String() string {
	var volSlice []string

	for _, volume := range *v {
		volSlice = append(volSlice, fmt.Sprintf("%s:%s", volume.MountTag, volume.HostPath))
	}

	return strings.Join(volSlice, " ")
}

// Socket defines a socket to communicate between
// the host and any process inside the VM.
type Socket struct {
	DeviceID string
	ID       string
	HostPath string
	Name     string
}

// Sockets is a Socket list.
type Sockets []Socket

// Set assigns socket values from string to a Socket.
func (s *Sockets) Set(sockStr string) error {
	sockSlice := strings.Split(sockStr, " ")

	for _, sock := range sockSlice {
		sockArgs := strings.Split(sock, ":")

		if len(sockArgs) != 4 {
			return fmt.Errorf("Wrong string format: %s, expecting only 4 parameters separated with ':'", sock)
		}

		for _, a := range sockArgs {
			if a == "" {
				return fmt.Errorf("Socket parameters cannot be empty")
			}
		}

		socket := Socket{
			DeviceID: sockArgs[0],
			ID:       sockArgs[1],
			HostPath: sockArgs[2],
			Name:     sockArgs[3],
		}

		*s = append(*s, socket)
	}

	return nil
}

// String converts a Socket to a string.
func (s *Sockets) String() string {
	var sockSlice []string

	for _, sock := range *s {
		sockSlice = append(sockSlice, fmt.Sprintf("%s:%s:%s:%s", sock.DeviceID, sock.ID, sock.HostPath, sock.Name))
	}

	return strings.Join(sockSlice, " ")
}

// EnvVar is a key/value structure representing a command
// environment variable.
type EnvVar struct {
	Var   string
	Value string
}

// Cmd represents a command to execute in a running container.
type Cmd struct {
	Args    []string
	Envs    []EnvVar
	WorkDir string

	User  string
	Group string
}

// Resources describes VM resources configuration.
type Resources struct {
	// VCPUs is the number of available virtual CPUs.
	VCPUs uint

	// Memory is the amount of available memory in MiB.
	Memory uint
}

// PodConfig is a Pod configuration.
type PodConfig struct {
	ID string

	// VMConfig is the VM configuration to set for this pod.
	VMConfig Resources

	HypervisorType   HypervisorType
	HypervisorConfig HypervisorConfig

	AgentType   AgentType
	AgentConfig interface{}

	NetworkModel  NetworkModel
	NetworkConfig NetworkConfig

	// Rootfs is the pod root file system in the host.
	// This can be left empty if we only have a set of containers
	// workload images and expect the agent to aggregate them into
	// a pod from the guest.
	RootFs string

	// Volumes is a list of shared volumes between the host and the Pod.
	Volumes []Volume

	// Containers describe the list of containers within a Pod.
	// This list can be empty and populated by adding containers
	// to the Pod a posteriori.
	Containers []ContainerConfig
}

// valid checks that the pod configuration is valid.
func (podConfig *PodConfig) valid() bool {
	newAgent(podConfig.AgentType)

	if _, err := newHypervisor(podConfig.HypervisorType); err != nil {
		podConfig.HypervisorType = QemuHypervisor
	}

	if podConfig.ID == "" {
		podConfig.ID = uuid.Generate().String()
	}

	return true
}

// lock locks any pod to prevent it from being accessed by other processes.
func lockPod(podID string) (*os.File, error) {
	fs := filesystem{}
	podlockFile, _, err := fs.podURI(podID, lockFileType)
	if err != nil {
		return nil, err
	}

	lockFile, err := os.Open(podlockFile)
	if err != nil {
		return nil, err
	}

	err = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX)
	if err != nil {
		return nil, err
	}

	return lockFile, nil
}

// unlock unlocks any pod to allow it being accessed by other processes.
func unlockPod(lockFile *os.File) error {
	err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
	if err != nil {
		return err
	}

	lockFile.Close()

	return nil
}

// Pod is composed of a set of containers and a runtime environment.
// A Pod can be created, deleted, started, stopped, listed, entered, paused and restored.
type Pod struct {
	id string

	hypervisor hypervisor
	agent      agent
	storage    resourceStorage

	config *PodConfig

	rootFs  string
	volumes []Volume

	containers []ContainerConfig

	runPath    string
	configPath string

	controlSocket string

	state State

	lockFile *os.File
}

// ID returns the pod identifier string.
func (p *Pod) ID() string {
	return p.id
}

func (p *Pod) createSetStates() error {
	err := p.setPodState(stateReady)
	if err != nil {
		return err
	}

	err = p.setContainersState(stateReady)
	if err != nil {
		return err
	}

	return nil
}

// createPod creates a pod from a pod description, the containers list, the hypervisor
// and the agent passed through the Config structure.
// It will create and store the pod structure, and then ask the hypervisor
// to physically create that pod i.e. starts a VM for that pod to eventually
// be started.
func createPod(podConfig PodConfig, netPairs []NetworkInterfacePair) (*Pod, []NetworkInterfacePair, error) {
	if podConfig.valid() == false {
		return nil, netPairs, fmt.Errorf("Invalid pod configuration")
	}

	agent := newAgent(podConfig.AgentType)

	hypervisor, err := newHypervisor(podConfig.HypervisorType)
	if err != nil {
		return nil, netPairs, err
	}

	err = hypervisor.init(podConfig.HypervisorConfig)
	if err != nil {
		return nil, netPairs, err
	}

	p := &Pod{
		id:         podConfig.ID,
		hypervisor: hypervisor,
		agent:      agent,
		storage:    &filesystem{},
		config:     &podConfig,
		rootFs:     podConfig.RootFs,
		volumes:    podConfig.Volumes,
		containers: podConfig.Containers,
		runPath:    filepath.Join(runStoragePath, podConfig.ID),
		configPath: filepath.Join(configStoragePath, podConfig.ID),
		state:      State{},
	}

	err = p.storage.createAllResources(*p)
	if err != nil {
		return nil, netPairs, err
	}

	err = p.hypervisor.createPod(podConfig)
	if err != nil {
		p.storage.deletePodResources(p.id, nil)
		return nil, netPairs, err
	}

	var agentConfig interface{}

	if podConfig.AgentConfig != nil {
		switch podConfig.AgentConfig.(type) {
		case (map[string]interface{}):
			agentConfig = newAgentConfig(podConfig)
		default:
			agentConfig = podConfig.AgentConfig.(interface{})
		}
	} else {
		agentConfig = nil
	}

	err = p.agent.init(*p, agentConfig)
	if err != nil {
		p.storage.deletePodResources(p.id, nil)
		return nil, netPairs, err
	}

	state, err := p.storage.fetchPodState(p.id)
	if err == nil && state.State != "" {
		return p, netPairs, nil
	}

	err = p.createSetStates()
	if err != nil {
		p.storage.deletePodResources(p.id, nil)
		return nil, netPairs, err
	}

	return p, netPairs, nil
}

// storePod stores a pod config.
func (p *Pod) storePod(netPairs []NetworkInterfacePair) error {
	fs := filesystem{}

	err := fs.storePodResource(p.id, configFileType, *(p.config))
	if err != nil {
		return err
	}

	for _, container := range p.containers {
		err = fs.storeContainerResource(p.id, container.ID, configFileType, container)
		if err != nil {
			return err
		}
	}

	// Store network pairs.
	err = fs.storePodResource(p.id, networkFileType, NetworkInterfacePairs(netPairs))
	if err != nil {
		return err
	}

	return nil
}

// fetchPod fetches a pod config from a pod ID and returns a pod.
func fetchPod(podID string) (*Pod, []NetworkInterfacePair, error) {
	var netPairs []NetworkInterfacePair

	fs := filesystem{}
	config, err := fs.fetchPodConfig(podID)
	if err != nil {
		return nil, netPairs, err
	}

	netPairs, err = fs.fetchPodNetwork(podID)
	if err != nil {
		return nil, netPairs, err
	}

	glog.Infof("Info structure:\n%+v\n", config)

	return createPod(config, netPairs)
}

// delete deletes an already created pod.
// The VM in which the pod is running will be shut down.
func (p *Pod) delete() error {
	state, err := p.storage.fetchPodState(p.id)
	if err != nil {
		return err
	}

	if state.State != stateReady {
		return fmt.Errorf("Pod not ready, impossible to delete")
	}

	err = p.storage.deletePodResources(p.id, nil)
	if err != nil {
		return err
	}

	return nil
}

func (p *Pod) startCheckStates() error {
	state, err := p.storage.fetchPodState(p.id)
	if err != nil {
		return err
	}

	err = state.validTransition(stateReady, stateRunning)
	if err != nil {
		return err
	}

	err = p.checkContainersState(stateReady)
	if err != nil {
		return err
	}

	return nil
}

func (p *Pod) startSetStates() error {
	err := p.setPodState(stateRunning)
	if err != nil {
		return err
	}

	err = p.setContainersState(stateRunning)
	if err != nil {
		return err
	}

	return nil
}

func (p *Pod) setNetwork() error {
	return nil
}

// start starts a pod. The containers that are making the pod
// will be started.
func (p *Pod) start() error {
	err := p.startCheckStates()
	if err != nil {
		return nil
	}

	podStartedCh := make(chan struct{})
	podStoppedCh := make(chan struct{})

	go p.hypervisor.startPod(podStartedCh, podStoppedCh)

	// Wait for the pod started notification
	select {
	case <-podStartedCh:
		break
	case <-time.After(time.Second):
		return fmt.Errorf("Did not receive the pod started notification")
	}

	err = p.agent.startAgent()
	if err != nil {
		p.stop()
		return err
	}

	err = p.agent.startPod(*p.config)
	if err != nil {
		p.stop()
		return err
	}

	interactive := false
	for _, c := range p.config.Containers {
		if c.Interactive != false && c.Console != "" {
			interactive = true
			break
		}
	}

	err = p.startSetStates()
	if err != nil {
		return err
	}

	if interactive == true {
		select {
		case <-podStoppedCh:
			err = p.stopSetStates()
			if err != nil {
				return err
			}

			break
		}
	} else {
		glog.Infof("Created Pod %s\n", p.ID())
	}

	return nil
}

func (p *Pod) stopCheckStates() error {
	err := p.checkContainersState(stateRunning)
	if err != nil {
		return err
	}

	state, err := p.storage.fetchPodState(p.id)
	if err != nil {
		return err
	}

	err = state.validTransition(stateRunning, stateReady)
	if err != nil {
		return err
	}

	return nil
}

func (p *Pod) stopSetStates() error {
	err := p.setContainersState(stateReady)
	if err != nil {
		return err
	}

	err = p.setPodState(stateReady)
	if err != nil {
		return err
	}

	return nil
}

// stop stops a pod. The containers that are making the pod
// will be destroyed.
func (p *Pod) stop() error {
	err := p.stopCheckStates()
	if err != nil {
		return err
	}

	err = p.agent.startAgent()
	if err != nil {
		return err
	}

	err = p.agent.stopPod(*p)
	if err != nil {
		return err
	}

	err = p.stopSetStates()
	if err != nil {
		return err
	}

	err = p.agent.stopAgent()
	if err != nil {
		return err
	}

	err = p.hypervisor.stopPod()
	if err != nil {
		return err
	}

	return nil
}

// list lists all pod running on the host.
func (p *Pod) list() ([]Pod, error) {
	return nil, nil
}

// enter runs an executable within a pod.
func (p *Pod) enter(args []string) error {
	return nil
}

func (p *Pod) setPodState(state stateString) error {
	p.state = State{
		State: state,
	}

	err := p.storage.storePodResource(p.id, stateFileType, p.state)
	if err != nil {
		return err
	}

	return nil
}

// endSession makes sure to end the session properly.
func (p *Pod) endSession() error {
	err := p.agent.stopAgent()
	if err != nil {
		return err
	}

	return nil
}

func (p *Pod) setContainerState(contID string, state stateString) error {
	contState := State{
		State: state,
	}

	err := p.storage.storeContainerResource(p.id, contID, stateFileType, contState)
	if err != nil {
		return err
	}

	return nil
}

func (p *Pod) setContainersState(state stateString) error {
	for _, container := range p.config.Containers {
		err := p.setContainerState(container.ID, state)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *Pod) deleteContainerState(contID string) error {
	err := p.storage.deleteContainerResources(p.id, contID, []podResource{stateFileType})
	if err != nil {
		return err
	}

	return nil
}

func (p *Pod) deleteContainersState() error {
	for _, container := range p.config.Containers {
		err := p.deleteContainerState(container.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *Pod) checkContainerState(contID string, expectedState stateString) error {
	state, err := p.storage.fetchContainerState(p.id, contID)
	if err != nil {
		return err
	}

	if state.State != expectedState {
		return fmt.Errorf("Container %s not %s", contID, expectedState)
	}

	return nil
}

func (p *Pod) checkContainersState(state stateString) error {
	for _, container := range p.config.Containers {
		err := p.checkContainerState(container.ID, state)
		if err != nil {
			return err
		}
	}

	return nil
}
