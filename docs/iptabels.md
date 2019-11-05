参考文章

1. [flannel 官方 issue: Failed to ensure iptables rules 的回复](https://github.com/coreos/flannel/issues/1159#issuecomment-511351680)

> flannel 代码中`main()`函数中对iptables的设置是在 flannel 容器中的操作, 且不影响宿主机上的防火墙规则.

在`main()`函数中, 构建Backend网络模型的操作与iptables操作是独立的, 不同网络模型对iptables的修改是相同的.

不过flannel容器使用的是宿主机的网络, 但是在容器内部对iptables的修改并不影响宿主机的iptables规则, 这一点还是比较神奇的, 需要研究一下.
