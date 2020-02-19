// Copyright 2015 flannel authors
// Licensed under the Apache License, Version 2.0 (the "License");
// +build !windows

package vxlan

// Some design notes and history:
// VXLAN encapsulates L2 packets (though flannel is L3 only so don't expect to be able to send L2 packets across hosts)
// VXLAN 对2层数据包进行了封装(但其实flannel只工作在3层, 所以不能跨主机发送2层包)
// VXLAN 是flannel对于kuber集群的默认网络模式(flannel的yaml默认写的就是vxlan).
// VTEP: Vxlan Tunnel End Point 的缩写
//
// The first versions of vxlan for flannel registered the flannel daemon as a handler for both "L2" and "L3" misses
// flannel vxlan的第一个版本将 flannel 注册为 2层和3层都未命中时的处理程序.
// - When a container sends a packet to a new IP address on the flannel network (but on a different host) this generates
//   an L2 miss (i.e. an ARP lookup)
//   一个容器向 flannel 网络中的一个新的IP(位于不同宿主机)发送一个数据包时, 就发生了 2层的miss(像ARP查询)
//   我们知道, arp只能在同一局域网(物理直连)有效, 不同的宿主机上部署docker环境, 即使两个docker的网段在同一子网下,
//   也无法实现跨宿主机的通信, 因为docker发的包没法通过物理网络嘛.
//   ...所以这就叫 L2 miss?
// - The flannel daemon knows which flannel host the packet is destined for so it can supply the VTEP MAC to use.
//   This is stored in the ARP table (with a timeout) to avoid constantly looking it up.
//   flannel 服务知道数据包要发送到哪一台主机上, 所以ta可以提供 VTEP MAC 来使用.
//   这些映射被存储在 arp 表中, 作为缓存保证不会被频繁查询.
// - The packet can then be encapsulated but the host needs to know where to send it.
//   This creates another callout from the kernal vxlan code to the flannel daemon
//   to get the public IP that should be used for that VTEP (this gets called an L3 miss).
//   The L2/L3 miss hooks are registered when the vxlan device is created.
//   At the same time a device route is created to the whole flannel network
//   so that non-local traffic is sent over the vxlan device.
//   然后可以对(容器发出的)数据包进行封装, 但宿主机起码要知道要发到哪.
//   为了能够知道在(宿主机)发出数据包时使用哪个IP,
//   处理 L2/L3 miss 的钩子在 vxlan 设备创建时被注册, 同时也会创建到 flannel 网络的设备路由,
//   这样 non-local(到其他主机,容器的请求) 流量将通过这个 vxlan 设备发送.
//
// In this scheme the scaling of table entries (per host) is:
// 在这个(vxlan)方案中, 将为每个宿主机做如下配置:
//  - 1 route (for the configured network out the vxlan device)
//    1条为 vxlan 设备配置的网络的出口路由
//  - One arp entry for each remote container that this host has recently contacted
//    为每个其他宿主机上的容器建立的 arp 记录())
//  - One FDB entry for each remote host
//    为每个其他宿主机建立的 FDB 记录
//
// The second version of flannel vxlan removed the need for the L3MISS callout.
// When a new remote host is found (either during startup or when it's created),
// flannel simply adds the required entries so that no further lookup/callout is required.
// flannel vxlan的第二个版本移除了处理 L3 miss 处理函数.
// 当发现一个新的其他宿主机加入 flannel 网络(不管是在ta创建还是在启动的时候),
// flannel 简单地把
//
// The latest version of the vxlan backend removes the need for the L2MISS too,
// which means that the flannel deamon is not listening for any netlink messages anymore.
// This improves reliability (no problems with timeouts if flannel crashes or restarts)
// and simplifies upgrades.
// vxlan 的最新版本把 L2 miss 的处理函数也移除了.
// 就是说, flannel 服务不再需要监听 netlink 信息.
// 这样提高了可靠性(如果 flannel 崩溃或重启, 将不再有超时问题), 且升级更简单.
//
// How it works:
// Create the vxlan device but don't register for any L2MISS or L3MISS messages
// Then, as each remote host is discovered (either on startup or when they are added),
// do the following
// 1) Create routing table entry for the remote subnet.
//    It goes via the vxlan device but also specifies a next hop (of the remote flannel host).
// 2) Create a static ARP entry for the remote flannel host IP address (and the VTEP MAC)
// 3) Create an FDB entry with the VTEP MAC and the public IP of the remote flannel daemon.
// 最新版本 vxlan 网络在创建 vxlan 设备时不需要注册 L2/L3 miss 事件处理程序.
// 因为其他宿主机在添加/启动的时候可以被自动发现.
// 1. 添加到其他宿主机子网(每个宿主机持有某一子网网段, 加入 flannel 时就会划分)的路由表记录.
//    数据包通过 vxlan 设备发出, 但需要指定下一跳(应该指的是宿主机所在网络的其他物理主机).
// 2. 为每个 flannel 网络中的其他宿主机创建一条静态的 arp 记录.
// 3. 为每个 flannel 网络中的其他宿主机创建
//
// In this scheme the scaling of table entries is linear to the number of remote hosts:
// 1 route, 1 arp entry and 1 FDB entry per host
// 在此方案中, 各表记录的修改都与 flannel 网络中主机数量成线性相关:
// 每增加一个节点就添加一条路由, 1条 arp 记录和1 条 FDB 记录.
//
// In this newest scheme, there is also the option of skipping the use of vxlan
// for hosts that are on the same subnet, this is called "directRouting"
// ...啥意思? 是指 flannel 划分到的容器网络与宿主机在同一网段吗? 直接路由???

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"

	"golang.org/x/net/context"

	"github.com/coreos/flannel/backend"
	"github.com/coreos/flannel/pkg/ip"
	"github.com/coreos/flannel/subnet"
)

func init() {
	backend.Register("vxlan", New)
}

const (
	defaultVNI = 1
)

type VXLANBackend struct {
	subnetMgr subnet.Manager
	extIface  *backend.ExternalInterface
}

func New(sm subnet.Manager, extIface *backend.ExternalInterface) (backend.Backend, error) {
	backend := &VXLANBackend{
		subnetMgr: sm,
		extIface:  extIface,
	}

	return backend, nil
}

// newSubnetAttrs 创建并返回新的 LeaseAttrs 租约属性对象.
// params:
//  * publicIP: 所在 node 的IP
//  * mac: flannel.x 的mac地址
func newSubnetAttrs(publicIP net.IP, mac net.HardwareAddr) (*subnet.LeaseAttrs, error) {
	data, err := json.Marshal(&vxlanLeaseAttrs{hardwareAddr(mac)})
	if err != nil {
		return nil, err
	}

	return &subnet.LeaseAttrs{
		PublicIP:    ip.FromIP(publicIP),
		BackendType: "vxlan",
		BackendData: json.RawMessage(data),
	}, nil
}

// RegisterNetwork ...
// caller: main.go -> main()
func (be *VXLANBackend) RegisterNetwork(
	ctx context.Context, 
	wg sync.WaitGroup, 
	config *subnet.Config,
) (backend.Network, error) {
	// Parse our configuration
	cfg := struct {
		VNI           int
		Port          int
		GBP           bool
		Learning      bool
		DirectRouting bool
	}{
		VNI: defaultVNI,
	}

	if len(config.Backend) > 0 {
		if err := json.Unmarshal(config.Backend, &cfg); err != nil {
			return nil, fmt.Errorf("error decoding VXLAN backend config: %v", err)
		}
	}
	// vetp 其实就指得是 vxlan 设备, index, addr, port等都是ta的属性.
	devAttrs := vxlanDeviceAttrs{
		vni:       uint32(cfg.VNI),
		name:      fmt.Sprintf("flannel.%v", cfg.VNI),
		vtepIndex: be.extIface.Iface.Index,
		vtepAddr:  be.extIface.IfaceAddr,
		vtepPort:  cfg.Port,
		gbp:       cfg.GBP,
		learning:  cfg.Learning,
	}
	// dev 即名称为 flannel.x 的网络接口
	dev, err := newVXLANDevice(&devAttrs)
	if err != nil {
		return nil, err
	}
	dev.directRouting = cfg.DirectRouting
	subnetAttrs, err := newSubnetAttrs(be.extIface.ExtAddr, dev.MACAddr())
	if err != nil {
		return nil, err
	}

	lease, err := be.subnetMgr.AcquireLease(ctx, subnetAttrs)
	switch err {
	case nil:
		// 这里什么也不做, 继续向下进行.
	case context.Canceled, context.DeadlineExceeded:
		// 超时/取消
		return nil, err
	default:
		return nil, fmt.Errorf("failed to acquire lease: %v", err)
	}
	// Ensure that the device has a /32 address
	// so that no broadcast routes are created.
	// This IP is just used as a source address for host to workload traffic
	// (so the return path for the traffic has an address
	// on the flannel network to use as the destination)
	//
	// 为 flannel.x 网络接口配置IP地址并启动.
	// lease.Subnet.IP 当前node节点划分到的网络号, 如 192.168.9.0/32.
	// 由于是ta的地址是网络号, 将不会再创建到网络号地址的路由.
	// 比如 eth0 的IP为 192.168.0.1/24, 将存在一个路由如下
	// 192.168.0.0/24 dev eth0 proto kernel scope link src 192.168.0.1 metric 100
	// 但 flannel.x 创建时不会有此记录.
	// 每个 flannel 网络中的节点都会有到其他节点网段的路由, 所以数据包可以正常发送与返回.
	if err := dev.Configure(ip.IP4Net{IP: lease.Subnet.IP, PrefixLen: 32}); err != nil {
		return nil, fmt.Errorf("failed to configure interface %s: %s", dev.link.Attrs().Name, err)
	}
	return newNetwork(be.subnetMgr, be.extIface, dev, ip.IP4Net{}, lease)
}

// So we can make it JSON (un)marshalable
type hardwareAddr net.HardwareAddr

func (hw hardwareAddr) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", net.HardwareAddr(hw))), nil
}

func (hw *hardwareAddr) UnmarshalJSON(bytes []byte) error {
	if len(bytes) < 2 || bytes[0] != '"' || bytes[len(bytes)-1] != '"' {
		return fmt.Errorf("error parsing hardware addr")
	}

	bytes = bytes[1 : len(bytes)-1]

	mac, err := net.ParseMAC(string(bytes))
	if err != nil {
		return err
	}

	*hw = hardwareAddr(mac)
	return nil
}
