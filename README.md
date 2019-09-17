
```
docker run -it --name flannel -v ~/go/src/github.com/coreos/flannel:/root/go/src/github.com/coreos/flannel generals/golang /bin/bash

ln -s /root/go/src/github.com/coreos/flannel /root/flannel
export http_proxy=http://192.168.124.85:1081
export https_proxy=http://192.168.124.85:1081
go get -v github.com/rogpeppe/godef
go get -v golang.org/x/tools/go/buildutil
go get -v github.com/ramya-rao-a/go-outline
```

## attach to running container

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

参考文章

1. [/usr/bin/ld: cannot find -lc 解决](http://blog.chinaunix.net/uid-31410005-id-5771901.html)
2. [Linux环境下gcc静态编译/usr/bin/ld: cannot find -lc错误原因及解决方法 ](https://www.xuebuyuan.com/3263655.html)

这其实是因为静态编译没有找到`.a`文件导致的, yum安装`glibc-static`可解决.
