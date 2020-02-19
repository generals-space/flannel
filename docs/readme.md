参考文章

1. [Flannel是如何工作的](https://cloud.tencent.com/developer/article/1096997)
    - vxlan, hostgw, udp才是有真正使用场景的网络模型, 其他都是实验性的, 不建议上生产.
    - [containernetworking/plugins/plugins/meta/flannel/README.md](https://github.com/containernetworking/plugins/blob/master/plugins/meta/flannel/README.md)工程才是真正的cni插件.

## 1. 核心概念

`backend`: 各种网络模型包括`vxlan`, `hostgw`, `udp`等, ta们在创建网络接口, 创建路由及ARP等操作上各有不同. 每种backend都需要实现`backend.Network`接口, 并在`Run()`函数中处理.

`etcd/kube manager`监听`node`节点的CURD事件, 然后由各`backend`处理, 主要就是为新节点划分网段, 添加对新节点子网的路由等, 或者回收旧节点的网段.

但是, 在`main()`中, `backend`的获取与注册, 与`iptables`的操作是独立的. 就是说, 不同的`backend`使用的是同一套`iptables`规则. 从更上层来看, 可以说`flannel`中的各`backend`提供的都是`overlay`网络方案, 借助NAT实现集群通信.

## 2. `coreos/flannel`与`cni/flannel`

不过`flannel`好像不管理Pod增删的网络部署操作, 只是为每个`node`划分了子网网段, Pod增删时由`kubelet`在CIDR范围内分配IP并创建路由.

这是怎么回事呢? 按照参考文章1所说, [coreos/flannel](https://github.com/coreos/flannel)其实并不是CNI插件, ta根本没有实现CNI的接口.

而且按照[containernetworking/plugins/plugins/meta/flannel/README.md](https://github.com/containernetworking/plugins/blob/master/plugins/meta/flannel/README.md)所说, 最终实现CNI插件功能的, 就是这里的`flannel`. 

在`yum`安装`kubernetes-cni`后, `/opt/cni/bin/`目录下会出现各种CNI插件. 这里的`flannel`只有2M左右, 与[coreos/flannel]不是同一个. [plugins/flannel]才会读取`/etc/cni/net.d/`目录下的CNI配置文件.

```console
[root@k8s-master-01 bin]# pwd
/opt/cni/bin
[root@k8s-master-01 bin]# ll -h
总用量 36M
drwxr-xr-x. 2 root root  195 1月  28 12:14 .
drwxr-xr-x. 3 root root   17 1月  28 12:13 ..
-rwxr-xr-x. 1 root root 2.9M 3月  26 2019 bridge
-rwxr-xr-x. 1 root root 7.3M 3月  26 2019 dhcp
-rwxr-xr-x. 1 root root 2.1M 3月  26 2019 flannel
...省略
```

