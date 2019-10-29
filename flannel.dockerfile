## docker build --no-cache=true -f flannel.dockerfile -t registry.cn-hangzhou.aliyuncs.com/generals-kuber/flannel:1.0.0 .
FROM registry.cn-hangzhou.aliyuncs.com/generals-kuber/flannel-base

COPY dist/flanneld /opt/bin/flanneld
COPY dist/mk-docker-opts.sh /opt/bin/

ENTRYPOINT ["/opt/bin/flanneld"]
