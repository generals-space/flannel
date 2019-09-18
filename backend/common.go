// Copyright 2015 flannel authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package backend

import (
	"net"
	"sync"

	"golang.org/x/net/context"

	"github.com/coreos/flannel/subnet"
)

type ExternalInterface struct {
	Iface     *net.Interface
	IfaceAddr net.IP // 接口上的IP, 一般是宿主机的IP
	ExtAddr   net.IP // ...好像和 IfaceAddr 一样?
}

// Backend ...
// Besides the entry points in the Backend interface, the backend's New()
// function receives static network interface information (like internal and
// external IP addresses, MTU, etc) which it should cache for later use if needed.
type Backend interface {
	// Called when the backend should create or begin managing a new network
	RegisterNetwork(ctx context.Context, wg sync.WaitGroup, config *subnet.Config) (Network, error)
}

// Network ...
// SimpleNetwork与RouteNetwork都实现了Network接口
type Network interface {
	Lease() *subnet.Lease
	MTU() int
	Run(ctx context.Context)
}

type BackendCtor func(sm subnet.Manager, ei *ExternalInterface) (Backend, error)
