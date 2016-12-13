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

// noopNetwork a.k.a. NO-OP Network is an empty network implementation, for
// testing and mocking purposes.
type noopNetwork struct {
}

// add creates a new network namespace and its virtual network interfaces,
// and it creates and bridges TAP interfaces for the Noop network.
// It does nothing.
func (n *noopNetwork) add(config *NetworkConfig) ([]NetworkInterfacePair, error) {
	return []NetworkInterfacePair{}, nil
}

// join switches the current process to the specified network namespace for
// the Noop network.
// It does nothing.
func (n *noopNetwork) join(config NetworkConfig) error {
	return nil
}

// remove unbridges and deletes TAP interfaces. It also removes virtual network
// interfaces and deletes the network namespace for the Noop network.
// It does nothing.
func (n *noopNetwork) remove(config NetworkConfig, netPairs []NetworkInterfacePair) error {
	return nil
}
