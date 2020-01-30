参考文章

1. [Flannel是如何工作的](https://cloud.tencent.com/developer/article/1096997)
    - vxlan, hostgw, udp才是有真正使用场景的网络模型, 其他都是实验性的, 不建议上生产.
    - [containernetworking/plugins/plugins/meta/flannel/README.md](https://github.com/containernetworking/plugins/blob/master/plugins/meta/flannel/README.md)工程才是真正的cni插件.

backend: 各种网络模型包括vxlan, hostgw等, ta们在创建网络接口, 创建路由及iptables规则等操作上各有不同.
backend network: 各backend对网络

每种backend都需要实现`backend.Network`接口, 并在`Run()`函数中处理.

由etcd/kube manager下发node的CURD事件, 然后由各backend处理, 主要就是为新节点划分网段, 或者回收旧节点的网段.

不过flannel好像不管理pod增删的网络部署操作, 只是为每个node划分了子网网段, pod增删时由kubelet在CIDR范围内分配IP并创建路由.

这是怎么回事呢? 按照[Flannel是如何工作的](https://cloud.tencent.com/developer/article/1096997)这篇文章所说, [coreos/flannel](https://github.com/coreos/flannel)其实并不是cni插件, ta根本没有实现cni的接口.

而且按照[containernetworking/plugins/plugins/meta/flannel/README.md](https://github.com/containernetworking/plugins/blob/master/plugins/meta/flannel/README.md)所说, 最终实现cni插件功能的, 就是这里的flannel. 在`yum`安装`kubernetes-cni`后, `/opt/cni/bin/`目录下会出现各种cni插件.

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

这里的`flannel`只有2M左右, 与[coreos/flannel]不是同一个. [plugins/flannel]才会读取`/etc/cni/net.d/`目录下的cni配置文件.
