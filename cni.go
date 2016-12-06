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
	"path/filepath"

	"github.com/containernetworking/cni/pkg/ns"
)

// cni is a network implementation for the CNI plugin.
type cni struct {
	config NetworkConfig
}

func (n *cni) addVirtInterfaces(netConfig NetworkConfig, netPairs []NetworkInterfacePair) error {
	_, nsName := filepath.Split(netConfig.NetNSPath)
	if nsName == "" {
		return fmt.Errorf("Invalid namespace name")
	}

	return nil
}

func (n *cni) deleteVirtInterfaces(netConfig NetworkConfig, netPairs []NetworkInterfacePair) error {
	return nil
}

// add creates a new network namespace and its virtual network interfaces,
// and it creates and bridges TAP interfaces for the CNI network.
func (n *cni) add(config *NetworkConfig) ([]NetworkInterfacePair, error) {
	var netPairs []NetworkInterfacePair
	var err error

	if config.NetNSPath == "" {
		path, err := createNetNS()
		if err != nil {
			return netPairs, err
		}

		config.NetNSPath = path
	}

	netPairs, err = createNetworkInterfacePairs(config.NumInterfaces)
	if err != nil {
		return netPairs, err
	}

	err = n.addVirtInterfaces(*config, netPairs)
	if err != nil {
		return netPairs, err
	}

	err = setNetNS(config.NetNSPath)
	if err != nil {
		return netPairs, err
	}

	for _, pair := range netPairs {
		err = bridgeNetworkPair(pair)
		if err != nil {
			return netPairs, err
		}
	}

	return netPairs, nil
}

// join switches the current process to the specified network namespace
// for the CNI network.
func (n *cni) join(config NetworkConfig) error {
	err := setNetNS(config.NetNSPath)
	if err != nil {
		return err
	}

	return nil
}

// remove unbridges and deletes TAP interfaces. It also removes virtual network
// interfaces and deletes the network namespace for the CNI network.
func (n *cni) remove(config NetworkConfig, netPairs []NetworkInterfacePair) error {
	err := doNetNS(config.NetNSPath, func(_ ns.NetNS) error {
		for _, pair := range netPairs {
			err := unBridgeNetworkPair(pair)
			if err != nil {
				return err
			}
		}

		return nil
	})

	err = n.deleteVirtInterfaces(config, netPairs)
	if err != nil {
		return err
	}

	err = deleteNetNS(config.NetNSPath, true)
	if err != nil {
		return err
	}

	return nil
}
