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
	"net"
	"os"

	"github.com/01org/ciao/ssntp/uuid"
	"github.com/containernetworking/cni/pkg/ns"
	"golang.org/x/sys/unix"
)

// NetworkInterface defines a network interface.
type NetworkInterface struct {
	Name     string
	HardAddr net.HardwareAddr
}

// NetworkInterfacePair defines a pair between TAP and virtual network interfaces.
type NetworkInterfacePair struct {
	ID        string
	Name      string
	VirtIface NetworkInterface
	TAPIface  NetworkInterface
}

// NetworkInterfacePairs defines a list of NetworkInterfacePair.
type NetworkInterfacePairs []NetworkInterfacePair

// NetworkConfig is the network configuration related to a network.
type NetworkConfig struct {
	NetNSPath     string
	NumInterfaces int
}

// NetworkModel describes the type of network specification.
type NetworkModel string

const (
	// NoopNetworkModel is the No-Op network.
	NoopNetworkModel NetworkModel = "noop"

	// CNINetworkModel is the CNI network.
	CNINetworkModel NetworkModel = "CNI"

	// CNMNetworkModel is the CNM network.
	CNMNetworkModel NetworkModel = "CNM"
)

// Set sets a network type based on the input string.
func (networkType *NetworkModel) Set(value string) error {
	switch value {
	case "noop":
		*networkType = NoopNetworkModel
		return nil
	case "CNI":
		*networkType = CNINetworkModel
		return nil
	case "CNM":
		*networkType = CNMNetworkModel
		return nil
	default:
		return fmt.Errorf("Unknown network type %s", value)
	}
}

// String converts a network type to a string.
func (networkType *NetworkModel) String() string {
	switch *networkType {
	case NoopNetworkModel:
		return string(NoopNetworkModel)
	case CNINetworkModel:
		return string(CNINetworkModel)
	case CNMNetworkModel:
		return string(CNMNetworkModel)
	default:
		return ""
	}
}

// newNetwork returns a network from a network type.
func newNetwork(networkType NetworkModel) network {
	switch networkType {
	case NoopNetworkModel:
		return &noopNetwork{}
	case CNINetworkModel:
		return &cni{}
	case CNMNetworkModel:
		return &cnm{}
	default:
		return &noopNetwork{}
	}
}

func createNetNS() (string, error) {
	n, err := ns.NewNS()
	if err != nil {
		return "", err
	}

	return n.Path(), nil
}

func setNetNS(netNSPath string) error {
	n, err := ns.GetNS(netNSPath)
	if err != nil {
		return err
	}

	return n.Set()
}

func doNetNS(netNSPath string, cb func(ns.NetNS) error) error {
	n, err := ns.GetNS(netNSPath)
	if err != nil {
		return err
	}

	return n.Do(cb)
}

func deleteNetNS(netNSPath string, mounted bool) error {
	n, err := ns.GetNS(netNSPath)
	if err != nil {
		return err
	}

	err = n.Close()
	if err != nil {
		return err
	}

	// This unmount part is supposed to be done in the cni/ns package, but the "mounted"
	// flag is not updated when retrieving NetNs handler from GetNS().
	if mounted {
		if err = unix.Unmount(netNSPath, unix.MNT_DETACH); err != nil {
			return fmt.Errorf("Failed to unmount namespace %s: %v", netNSPath, err)
		}
		if err := os.RemoveAll(netNSPath); err != nil {
			return fmt.Errorf("Failed to clean up namespace %s: %v", netNSPath, err)
		}
	}

	return nil
}

func createNetworkInterfacePairs(numOfPairs int) ([]NetworkInterfacePair, error) {
	var netPairs []NetworkInterfacePair

	if numOfPairs < 1 {
		return netPairs, fmt.Errorf("Invalid number of network pairs")
	}

	uniqueID := uuid.Generate().String()

	for i := 0; i < numOfPairs; i++ {
		hardAddr := []byte{0x02, 0x00, 0xCA, 0xFE, byte(i >> 8), byte(i)}

		pair := NetworkInterfacePair{
			ID:   fmt.Sprintf("%s-%d", uniqueID, i),
			Name: fmt.Sprintf("br%d", i),
			VirtIface: NetworkInterface{
				Name:     fmt.Sprintf("eth%d", i),
				HardAddr: hardAddr,
			},
			TAPIface: NetworkInterface{
				Name: fmt.Sprintf("tap%d", i),
			},
		}

		netPairs = append(netPairs, pair)
	}

	return netPairs, nil
}

// network is the virtcontainers network interface.
// Container network plugins are used to setup virtual network
// between VM netns and the host network physical interface.
type network interface {
	// add creates a new network namespace and its virtual network interfaces,
	// and it creates and bridges TAP interfaces.
	add(config *NetworkConfig) ([]NetworkInterfacePair, error)

	// join switches the current process to the specified network namespace.
	join(config NetworkConfig) error

	// remove unbridges and deletes TAP interfaces. It also removes virtual network
	// interfaces and deletes the network namespace.
	remove(config NetworkConfig, netPairs []NetworkInterfacePair) error
}
