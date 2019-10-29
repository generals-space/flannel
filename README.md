创建容器, 用于编译flannel, 同时用于Mac平台通过vscode远程开发.

```
docker run -it --name flannel -v ~/go/src/github.com/generals-space/flannel:/root/go/src/github.com/coreos/flannel registry.cn-hangzhou.aliyuncs.com/generals-space/golang-rc /bin/bash
```

然后vscode连接到正在运行的容器(需要`Remote - Containers`插件)

## 编译

不用下载依赖, 都在vendor目录下.

进入容器执行`make dist/flanneld`进行编译会出错.

```
[root@6e48ee2d3b63 flannel]# make dist/flanneld
go build -o dist/flanneld \
  -ldflags '-s -w -X github.com/coreos/flannel/version.Version=v0.11.0-34-gecb6db3-dirty -extldflags "-static"'
# github.com/coreos/flannel
/usr/local/go/pkg/tool/linux_amd64/link: running gcc failed: exit status 1
/usr/bin/ld: cannot find -lpthread
/usr/bin/ld: cannot find -lc
collect2: error: ld returned 1 exit status

make: *** [dist/flanneld] Error 2
```

解决方法

```
yum install -y glibc-static
```

## 部署

flannel-base, flannel的dockerfile用于构建flannel镜像, 在kube-flannel.yml中使用, 而实际的编译操作则需要在golang-rc容器中完成.
