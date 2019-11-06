// Copyright 2015 flannel authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// +build !windows

package vxlan

import (
	"encoding/json"
	"net"
	"sync"

	log "github.com/golang/glog"
	"github.com/vishvananda/netlink"
	"golang.org/x/net/context"

	"syscall"

	"github.com/coreos/flannel/backend"
	"github.com/coreos/flannel/pkg/ip"
	"github.com/coreos/flannel/subnet"
)

type network struct {
	backend.SimpleNetwork
	dev       *vxlanDevice
	subnetMgr subnet.Manager
}

const (
	encapOverhead = 50
)

// newNetwork ...
//
// params:
//  * extIface: node节点上的网络接口
//  * dev: flannel.x 设备对象
func newNetwork(subnetMgr subnet.Manager, extIface *backend.ExternalInterface, dev *vxlanDevice, _ ip.IP4Net, lease *subnet.Lease) (*network, error) {
	nw := &network{
		SimpleNetwork: backend.SimpleNetwork{
			SubnetLease: lease,
			ExtIface:    extIface,
		},
		subnetMgr: subnetMgr,
		dev:       dev,
	}

	return nw, nil
}

// Run Backend网络模型构建完成, 且iptables规则写入后, 执行此函数正式启动.
// 无限循环处理 subnet.Event 事件.
// caller: main.go -> main()
func (nw *network) Run(ctx context.Context) {
	wg := sync.WaitGroup{}

	log.V(0).Info("watching for new subnet leases")
	events := make(chan []subnet.Event)
	wg.Add(1)
	go func() {
		// WatchLeases() 里是一个无限循环
		subnet.WatchLeases(ctx, nw.subnetMgr, nw.SubnetLease, events)
		log.V(1).Info("WatchLeases exited")
		wg.Done()
	}()

	defer wg.Wait()

	for {
		select {
		case evtBatch := <-events:
			nw.handleSubnetEvents(evtBatch)

		case <-ctx.Done():
			return
		}
	}
}

func (nw *network) MTU() int {
	return nw.ExtIface.Iface.MTU - encapOverhead
}

type vxlanLeaseAttrs struct {
	VtepMAC hardwareAddr
}

func (nw *network) handleSubnetEvents(batch []subnet.Event) {
	var err error
	for _, event := range batch {
		sn := event.Lease.Subnet
		attrs := event.Lease.Attrs
		if attrs.BackendType != "vxlan" {
			log.Warningf("ignoring non-vxlan subnet(%s): type=%v", sn, attrs.BackendType)
			continue
		}

		var vxlanAttrs vxlanLeaseAttrs
		err = json.Unmarshal(attrs.BackendData, &vxlanAttrs)
		if err != nil {
			log.Error("error decoding subnet lease JSON: ", err)
			continue
		}

		// This route is used when traffic should be vxlan encapsulated
		// flannel 容器使用的是宿主机的网络, 网络接口, 路由等都是一样的.
		// ...除了 iptables.
		vxlanRoute := netlink.Route{
			LinkIndex: nw.dev.link.Attrs().Index,
			Scope:     netlink.SCOPE_UNIVERSE,
			Dst:       sn.ToIPNet(),
			Gw:        sn.IP.ToIP(),
		}
		// 当 RTNH_F_ONLINK 标识被设置时, 要求内核不检查下一跳地址是否相连.
		// 即不检查下一跳地址通过流出设备是否可达
		vxlanRoute.SetFlag(syscall.RTNH_F_ONLINK)

		// directRouting is where the remote host is on the same subnet
		// so vxlan isn't required.
		directRoute := netlink.Route{
			Dst: sn.ToIPNet(),
			Gw:  attrs.PublicIP.ToIP(),
		}
		var directRoutingOK = false
		if nw.dev.directRouting {
			if dr, err := ip.DirectRouting(attrs.PublicIP.ToIP()); err != nil {
				log.Error(err)
			} else {
				directRoutingOK = dr
			}
		}

		log.Infof("=== direct routing: %t, handle subnet event: %d", directRoutingOK, event.Type)
		var n neighbor
		switch event.Type {
		case subnet.EventAdded:
			// EventAdded 表示新增 flannel 节点事件, 尝试将新增节点的ip写到路由中.
			// attrs.PublicIP 表示新节点的对外IP(出口IP)
			// vxlanAttrs.VtepMAC 表示新节点 flannel.1 的 MAC 地址
			if directRoutingOK {
				log.Infof("Adding direct route to subnet: %s PublicIP: %s", sn, attrs.PublicIP)

				if err := netlink.RouteReplace(&directRoute); err != nil {
					log.Errorf("Error adding route to %v via %v: %v", sn, attrs.PublicIP, err)
					continue
				}
			} else {
				log.Infof("adding subnet: %s PublicIP: %s VtepMAC: %s", 
					sn, attrs.PublicIP, net.HardwareAddr(vxlanAttrs.VtepMAC))
				
				n = neighbor{
					IP: sn.IP, 
					MAC: net.HardwareAddr(vxlanAttrs.VtepMAC),
				}
				err = nw.dev.AddARP(n);
				if err != nil {
					log.Error("AddARP failed: ", err)
					continue
				}
				n = neighbor{
					IP: attrs.PublicIP, 
					MAC: net.HardwareAddr(vxlanAttrs.VtepMAC),
				}
				err = nw.dev.AddFDB(n);
				if err != nil {
					log.Error("AddFDB failed: ", err)

					// Try to clean up the ARP entry then continue
					n = neighbor{
						IP: event.Lease.Subnet.IP, 
						MAC: net.HardwareAddr(vxlanAttrs.VtepMAC),
					}
					err = nw.dev.DelARP(n)
					if err != nil {
						log.Error("DelARP failed: ", err)
					}

					continue
				}

				// Set the route - the kernel would ARP for the Gw IP address 
				// if it hadn't already been set above so make sure this is done last.
				err = netlink.RouteReplace(&vxlanRoute)
				if err != nil {
					log.Errorf("failed to add vxlanRoute (%s -> %s): %v", vxlanRoute.Dst, vxlanRoute.Gw, err)

					// Try to clean up both the ARP and FDB entries then continue
					n = neighbor{
						IP: event.Lease.Subnet.IP, 
						MAC: net.HardwareAddr(vxlanAttrs.VtepMAC),
					}
					err = nw.dev.DelARP(n)
					if err != nil {
						log.Error("DelARP failed: ", err)
					}
					n = neighbor{
						IP: event.Lease.Attrs.PublicIP, 
						MAC: net.HardwareAddr(vxlanAttrs.VtepMAC),
					}
					err = nw.dev.DelFDB(n)
					if err != nil {
						log.Error("DelFDB failed: ", err)
					}

					continue
				}
			}
		case subnet.EventRemoved:
			if directRoutingOK {
				log.Infof("Removing direct route to subnet: %s PublicIP: %s", sn, attrs.PublicIP)
				err = netlink.RouteDel(&directRoute)
				if err != nil {
					log.Errorf("Error deleting route to %v via %v: %v", sn, attrs.PublicIP, err)
				}
			} else {
				log.Infof("removing subnet: %s PublicIP: %s VtepMAC: %s", 
					sn, attrs.PublicIP, net.HardwareAddr(vxlanAttrs.VtepMAC))

				// Try to remove all entries - don't bail out if one of them fails.
				n = neighbor{
					IP: sn.IP, 
					MAC: net.HardwareAddr(vxlanAttrs.VtepMAC),
				}
				err = nw.dev.DelARP(n)
				if err != nil {
					log.Error("DelARP failed: ", err)
				}
				n = neighbor{
					IP: attrs.PublicIP, 
					MAC: net.HardwareAddr(vxlanAttrs.VtepMAC),
				}
				err = nw.dev.DelFDB(n)
				if err != nil {
					log.Error("DelFDB failed: ", err)
				}
				err = netlink.RouteDel(&vxlanRoute)
				if err != nil {
					log.Errorf("failed to delete vxlanRoute (%s -> %s): %v", 
						vxlanRoute.Dst, vxlanRoute.Gw, err)
				}
			}
		default:
			log.Error("internal error: unknown event type: ", int(event.Type))
		}
	}
}
