// Copyright 2015 flannel authors
//
// Licensed under the Apache License, Version 2.0 (the "License");

package backend

import (
	"fmt"
	"strings"
	"sync"

	"golang.org/x/net/context"

	"github.com/coreos/flannel/subnet"
	log "github.com/golang/glog"
)

// 各backend组件的名称与ta们构造函数的映射表.
// 注意ta与 manager.active 成员的关系.
var constructors = make(map[string]BackendCtor)

// Manager 该接口的对象只有一个GetBackend的功能, 就是为了看看当前使用的是哪种网络模型.
// 注意: 这里的 Manager 接口与 subnet/subnet.go 中的 Manager 接口不同.
type Manager interface {
	GetBackend(backendType string) (Backend, error)
}

type manager struct {
	ctx      context.Context
	sm       subnet.Manager
	extIface *ExternalInterface
	mux      sync.Mutex
	active   map[string]Backend
	wg       sync.WaitGroup
}

// NewManager 只是一个结构体的构造函数, 没有特别的地方.
// caller: main.go -> main()
func NewManager(ctx context.Context, sm subnet.Manager, extIface *ExternalInterface) Manager {
	return &manager{
		ctx:      ctx,
		sm:       sm,
		extIface: extIface,
		active:   make(map[string]Backend),
	}
}

// GetBackend 创建并返回 Backend 对象.
// 且与 manager 类型(kuber/etcd local) 无关.
// 可以说, Backend 对象就表示某种网络模型, 比如 udp, vxlan 等.
// 实际上, Backend 对象的构建都是在各网络模型的 New() 函数中完成.
// caller: main.go -> main()
func (bm *manager) GetBackend(backendType string) (Backend, error) {
	bm.mux.Lock()
	defer bm.mux.Unlock()

	betype := strings.ToLower(backendType)
	// 注意 active 的类型, 其中存储的是网络模型名称与对象的映射.
	// 这里先查看是否已经存在了 betype 所表示的类型的 Backend 对象.
	if be, ok := bm.active[betype]; ok {
		return be, nil
	}

	// first request, need to create and run it
	befunc, ok := constructors[betype]
	if !ok {
		return nil, fmt.Errorf("unknown backend type: %v", betype)
	}

	log.Infof("=========== In GetBackend(), Backend.subnetMgr: %+v, Backend.extIface: %+v", bm.sm, bm.extIface)

	be, err := befunc(bm.sm, bm.extIface)
	if err != nil {
		return nil, err
	}
	bm.active[betype] = be

	bm.wg.Add(1)
	go func() {
		<-bm.ctx.Done()

		// TODO(eyakubovich): this obviosly introduces a race.
		// GetBackend() could get called while we are here.
		// Currently though, all backends' Run exit only on shutdown

		bm.mux.Lock()
		delete(bm.active, betype)
		bm.mux.Unlock()

		bm.wg.Done()
	}()

	return be, nil
}

// Register 建立各 backend 组件名称与各包中 New() 函数的关联.
// 各backend组件通过调用此函数完成注册.
// name: backend 各自的名称, 如udp, vxlan等.
// ctor: constructor的缩写, 是ta们各自的构造函数 New().
func Register(name string, ctor BackendCtor) {
	constructors[name] = ctor
}
