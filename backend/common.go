// Copyright 2015 flannel authors
//
// Licensed under the Apache License, Version 2.0 (the "License");

package backend

import (
	"net"
	"sync"

	"golang.org/x/net/context"

	"github.com/coreos/flannel/subnet"
)

// ExternalInterface ...
// backend/manager.go -> manager struct{} 结构的 extIface 成员,
// 各网络模型实现的 Backend 对象的 exIface 成员,
// 都是这个类型.
// IfaceAddr 与 ExtAddr, 我所见过的情况中, ta俩的值都是相同的.
// 应该是 IfaceAddr 表示节点本身使用的地址, 
// 而 ExtAddr 是对于其他节点来说的地址吧, ??? 
// 与 etcd 集群中的 `advertise-client-urls` 类似.
type ExternalInterface struct {
	Iface     *net.Interface
	IfaceAddr net.IP // 接口上的IP, 一般是宿主机的IP
	ExtAddr   net.IP // ...好像和 IfaceAddr 一样?
}

// Backend ...
// Besides the entry points in the Backend interface, 
// the backend's New() function receives static network interface information 
// (like internal and external IP addresses, MTU, etc) 
// which it should cache for later use if needed.
// 
// 可以说, Backend 对象就表示某种网络模型, 比如 udp, vxlan 等.
// Backend 就只有 RegisterNetwork() 一个作用, 用于构建各种模型的网络. 
// 但 Backend 的对象必然是一个结构体, 其中保存着内部/外部的 IP, MTU 等信息.
type Backend interface {
	// Called when the backend should create or begin managing a new network
	RegisterNetwork(ctx context.Context, wg sync.WaitGroup, config *subnet.Config) (Network, error)
}

// Network SimpleNetwork与RouteNetwork都实现了Network接口, 
// 实际上各种网络模型(hostgw, ipip, vxlan)最终都会实现该接口, 不过一般是基于上面两种结构.
type Network interface {
	Lease() *subnet.Lease
	MTU() int
	// Run() 这是一个阻塞函数.
	Run(ctx context.Context)
}

// BackendCtor 即是各 Backend 组件的 New() 构造函数.
type BackendCtor func(sm subnet.Manager, ei *ExternalInterface) (Backend, error)
