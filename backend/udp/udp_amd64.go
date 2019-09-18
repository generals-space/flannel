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
// +build !windows

package udp

import (
	"encoding/json"
	"fmt"
	"sync"

	"golang.org/x/net/context"

	"github.com/coreos/flannel/backend"
	"github.com/coreos/flannel/pkg/ip"
	"github.com/coreos/flannel/subnet"
)

func init() {
	backend.Register("udp", New)
}

const (
	defaultPort = 8285
)

type UdpBackend struct {
	sm       subnet.Manager
	extIface *backend.ExternalInterface
}

func New(sm subnet.Manager, extIface *backend.ExternalInterface) (backend.Backend, error) {
	be := UdpBackend{
		sm:       sm,
		extIface: extIface,
	}
	return &be, nil
}

func (be *UdpBackend) RegisterNetwork(ctx context.Context, wg sync.WaitGroup, config *subnet.Config) (backend.Network, error) {
	cfg := struct {
		Port int
	}{
		Port: defaultPort,
	}

	// Parse our configuration
	// 解析 udp 模型相关的配置
	if len(config.Backend) > 0 {
		if err := json.Unmarshal(config.Backend, &cfg); err != nil {
			return nil, fmt.Errorf("error decoding UDP backend config: %v", err)
		}
	}

	// Acquire the lease form subnet manager
	attrs := subnet.LeaseAttrs{
		PublicIP: ip.FromIP(be.extIface.ExtAddr),
	}

	l, err := be.sm.AcquireLease(ctx, &attrs)
	switch err {
	case nil:
		// 这里什么都不做.
	case context.Canceled, context.DeadlineExceeded:
		return nil, err
	default:
		return nil, fmt.Errorf("failed to acquire lease: %v", err)
	}

	// Tunnel's subnet is that of the whole overlay network (e.g. /16)
	// and not that of the individual host (e.g. /24)
	// tunnel 的子网范围是掩码16, 是整个虚拟网段, 而不是24(单个主机的容器网段)
	tunNet := ip.IP4Net{
		IP:        l.Subnet.IP,
		PrefixLen: config.Network.PrefixLen, // 网段前缀长度, 这里应该是16
	}

	return newNetwork(be.sm, be.extIface, cfg.Port, tunNet, l)
}
