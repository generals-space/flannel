// Copyright 2015 flannel authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// +build !windows

package network

import (
	"fmt"
	"strings"

	log "github.com/golang/glog"

	"time"

	"github.com/coreos/flannel/pkg/ip"
	"github.com/coreos/flannel/subnet"
	"github.com/coreos/go-iptables/iptables"
)

// IPTables 这里为什么要声明接口呢? 直接使用 iptables 包中的数据结构及函数不可以吗???
type IPTables interface {
	AppendUnique(table string, chain string, rulespec ...string) error
	Delete(table string, chain string, rulespec ...string) error
	Exists(table string, chain string, rulespec ...string) (bool, error)
	List(table, chain string) ([]string, error)
}

type IPTablesRule struct {
	table    string
	chain    string
	rulespec []string
}

// MasqRules 生成并返回nat表规则(masquerade操作), 貌似全都是`POSTROUTING`链规则.
/*
-t nat -A POSTROUTING -s 大子网/16 -d 大子网/16 -j RETURN
-t nat -A POSTROUTING -s 大子网/16 ! -d 224.0.0.0/4 -j MASQUERADE
-t nat -A POSTROUTING ! -s 大子网/16 -d 小子网/24 -j RETURN
-t nat -A POSTROUTING ! -s 大子网/16 -d 大子网/16 -j MASQUERADE
*/
func MasqRules(flannelNetwork, subNetwork string) []IPTablesRule {
	supports_random_fully := false
	ipt, err := iptables.New()
	if err == nil {
		supports_random_fully = ipt.HasRandomFully()
	}
	// 两种情况只有两处不同, 区别只在于在规则末尾追加了 --random-fully 选项.
	if supports_random_fully {
		return []IPTablesRule{
			// This rule makes sure we don't NAT traffic within overlay network
			// (e.g. coming out of docker0)
			{"nat", "POSTROUTING", []string{
				"-s", flannelNetwork, "-d", flannelNetwork, 
				"-j", "RETURN",
			},
			},
			// NAT if it's not multicast traffic
			{"nat", "POSTROUTING", []string{
				"-s", flannelNetwork, "!", "-d", "224.0.0.0/4",
				"-j", "MASQUERADE", "--random-fully",
			},
			},
			// Prevent performing Masquerade on external traffic
			// which arrives from a Node that owns the container/pod IP address
			{"nat", "POSTROUTING", []string{
				"!", "-s", flannelNetwork, "-d", subNetwork, "-j", "RETURN",
			},
			},
			// Masquerade anything headed towards flannel from the host
			{"nat", "POSTROUTING", []string{
				"!", "-s", flannelNetwork, "-d", flannelNetwork,
				"-j", "MASQUERADE", "--random-fully",
			},
			},
		}
	} else {
		return []IPTablesRule{
			// This rule makes sure we don't NAT traffic within overlay network
			// (e.g. coming out of docker0)
			// 大子网之间, 即Pod之间的通信, 不需要进行NAT转换.
			{"nat", "POSTROUTING", []string{
				"-s", flannelNetwork, "-d", flannelNetwork, 
				"-j", "RETURN",
			},
			},
			// NAT if it's not multicast traffic
			// 从大子网内, 即从Pod内部发出的请求, 而目标地址不是多播地址的请求.
			// 多播的话其实仍然是Pod之间的通信, vxlan模型就需要用到多播协议.
			// 那么只有可能是访问宿主机, 或是访问外网了, 对这样的请求, 需要MASQUERADE做SNAT转换.
			{"nat", "POSTROUTING", []string{
				"-s", flannelNetwork, "!", "-d", "224.0.0.0/4",
				"-j", "MASQUERADE",
			},
			},
			// Prevent performing Masquerade on external traffic
			// which arrives from a Node that owns the container/pod IP address
			// 来自外部网络的, 且目标地址是当前节点上小子网的请求, 不需要进行NAT.
			// 由于外部网络不会知道Pod内部的IP地址(毕竟内网地址无法被路由), 
			// 所以基本可以认为这是从宿主机节点发起的, 目标是本地上的Pod的请求.
			{"nat", "POSTROUTING", []string{
				"!", "-s", flannelNetwork, "-d", subNetwork, "-j", "RETURN",
			},
			},
			// Masquerade anything headed towards flannel from the host
			// 来自外部网络的, 但目标地址是其他节点上的子网的请求, 
			// 这种一般是从当前宿主机节点发起的, 目标地址是其他节点上的Pod的请求.
			// 需要进行NAT的.
			{"nat", "POSTROUTING", []string{
				"!", "-s", flannelNetwork, "-d", flannelNetwork,
				"-j", "MASQUERADE",
			},
			},
		}
	}
}

// ForwardRules 构建 forward 形式的 iptables 规则数组并返回.
// caller: main.go -> main()
/*
FORWARD 处于 PREROUTING 之后, POSTROUTEING 之前, 所以如果希望上面的NAT规则生效, 首先要保证流经 FORWARD 链的数据包被接受.
-t fileter -A FORWARD -s 大子网/16 -j ACCEPT
-t fileter -A FORWARD -d 大子网/16 -j ACCEPT
*/
func ForwardRules(flannelNetwork string) []IPTablesRule {
	return []IPTablesRule{
		// These rules allow traffic to be forwarded
		// if it is to or from the flannel network range.
		// 注意: iptables 默认的那个表就是 filter 表, 另外两个是 nat 和 mangle.
		// 所以下面两条规则就是允许/接受 "来自或去向 flannel 网络(即各Pod所在的子网)的数据包".
		// 本质上就是允许当前节点作为路由, 转发.
		{"filter", "FORWARD", []string{"-s", flannelNetwork, "-j", "ACCEPT"}},
		{"filter", "FORWARD", []string{"-d", flannelNetwork, "-j", "ACCEPT"}},
	}
}

// ipTablesRulesExist 判断 rules 数组中的规则都存在.
// 注意: 需要 rules 中所有规则, 假如有一条不存在, 都会报错.
// caller: ensureIPTables()
func ipTablesRulesExist(ipt IPTables, rules []IPTablesRule) (bool, error) {
	for _, rule := range rules {
		exists, err := ipt.Exists(rule.table, rule.chain, rule.rulespec...)
		if err != nil {
			// this shouldn't ever happen
			return false, fmt.Errorf("failed to check rule existence: %v", err)
		}
		if !exists {
			return false, nil
		}
	}

	return true, nil
}

// SetupAndEnsureIPTables 设置 rules 数组所表示的规则,
// 同时维护一个 for{} 无限循环, 每隔 resyncPeriod 秒检测并确认 rules 规则仍存在.
// caller: main.go -> main(), 只有这一处.
// 通过 go func() 形式启动.
func SetupAndEnsureIPTables(rules []IPTablesRule, resyncPeriod int) {
	ipt, err := iptables.New()
	if err != nil {
		log.Errorf("Failed to setup IPTables. iptables binary was not found: %v", err)
		return
	}

	defer func() {
		teardownIPTables(ipt, rules)
	}()

	for {
		// Ensure that all the iptables rules exist every resyncPeriod seconds
		// 每隔 resyncPeriod 秒确认一次 rules 规则存在.
		if err := ensureIPTables(ipt, rules); err != nil {
			log.Errorf("Failed to ensure iptables rules: %v", err)
		}

		time.Sleep(time.Duration(resyncPeriod) * time.Second)
	}
}

// DeleteIPTables delete specified iptables rules
func DeleteIPTables(rules []IPTablesRule) error {
	ipt, err := iptables.New()
	if err != nil {
		log.Errorf("Failed to setup IPTables. iptables binary was not found: %v", err)
		return err
	}
	teardownIPTables(ipt, rules)
	return nil
}

// ensureIPTables 确认 rules 数组中的规则存在.
// caller: SetupAndEnsureIPTables()
func ensureIPTables(ipt IPTables, rules []IPTablesRule) error {
	exists, err := ipTablesRulesExist(ipt, rules)
	if err != nil {
		return fmt.Errorf("Error checking rule existence: %v", err)
	}
	if exists {
		// if all the rules already exist, no need to do anything
		return nil
	}
	// Otherwise, teardown all the rules and set them up again
	// We do this because the order of the rules is important
	log.Info("Some iptables rules are missing; deleting and recreating rules")
	teardownIPTables(ipt, rules)
	if err = setupIPTables(ipt, rules); err != nil {
		return fmt.Errorf("Error setting up rules: %v", err)
	}
	return nil
}

// setupIPTables 将 rules 规则数组追加到各表各链的尾部.
// caller: ensureIPTables()
func setupIPTables(ipt IPTables, rules []IPTablesRule) error {
	for _, rule := range rules {
		log.Info("Adding iptables rule: ", strings.Join(rule.rulespec, " "))
		err := ipt.AppendUnique(rule.table, rule.chain, rule.rulespec...)
		if err != nil {
			return fmt.Errorf("failed to insert IPTables rule: %v", err)
		}
	}

	return nil
}

// teardownIPTables 删除 rules 数组中的规则.
// caller: SetupAndEnsureIPTables(), DeleteIPTables(), ensureIPTables()
func teardownIPTables(ipt IPTables, rules []IPTablesRule) {
	for _, rule := range rules {
		log.Info("Deleting iptables rule: ", strings.Join(rule.rulespec, " "))
		// We ignore errors here because if there's an error
		// it's almost certainly because the rule doesn't exist,
		// which is fine (we don't need to delete rules that don't exist)
		// 这里可以忽略 Delete 的错误, 因为出现 error 时,
		// 几乎可以确定就是因为目标规则不存在, 没关系.
		ipt.Delete(rule.table, rule.chain, rule.rulespec...)
	}
}
