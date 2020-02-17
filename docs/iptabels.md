参考文章

1. [flannel 官方 issue: Failed to ensure iptables rules 的回复](https://github.com/coreos/flannel/issues/1159#issuecomment-511351680)

> flannel 代码中`main()`函数中对iptables的设置是在 flannel 容器中的操作, 且不影响宿主机上的防火墙规则.

在`main()`函数中, 构建Backend网络模型的操作与iptables操作是独立的, 不同网络模型对iptables的修改是相同的.

不过flannel容器使用的是宿主机的网络, 但是在容器内部对iptables的修改并不影响宿主机的iptables规则, 这一点还是比较神奇的, 需要研究一下.

------

calico/flannel在部署完成后只改写自己本身Pod的`iptables`规则, 虽然ta们的Pod用的是`hostNetwork`, 但是在宿主机上是看不到这些规则的. 在kuber集群中, 所有使用`hostNetwork`的Pod的网络命名空间, 都通过Pause容器做了一个中转. 

> 实验时要指定一个Pod使用`hostNetwork`, 查看`iptables`规则还需要`privileged`权限哦.

所有使用`hostNetwork`的Pod, 其本身的容器使用的网络(docker本身有4种网络模型)的`container`类型, 且指定其对应的`Pause`容器, 而该`Pause`容器所使用的网络则是`host`.

```
d inspect -f '{{.HostConfig.NetworkMode}}' 容器ID或名称
```

> 使用`kubectl describe pod`是没办法看到这样的联系的, 只能使用`docker inspect`查看.

上面说了, 所有使用`hostNetwork`的Pod的网络空间是相连的, 最终都指向`host`, 但那是通过`kubectl exec`或是`docker exec`进入到容器终端查看的. 实际上如果你通过`nsenter`直接进入ta们(不管是测试容器, 还是对应的`Pause`容器)的网络空间, 使用`iptables -nvL`命令会发现, `Pause`容器的iptables规则与宿主机本身的是相同的.

就是说, 使用`docker exec`进入容器命令行, 与`nsenter`直接进入网络空间, 看到的iptables规则是不相同的...

------

md, 在某一节点上使用`docker run --net host --privileged`启动容器竟然看到了与flannel容器内部相同的规则, 与宿主机的iptables不一样. 

但是我在非kuber集群上启动这样的容器, 内部看到的iptables与宿主机是相同且可以互相修改的.

...那就只能怀疑是不是kuber是否有某个组件保护了host的网络空间了...kubelet???
