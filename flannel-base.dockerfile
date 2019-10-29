## docker build --no-cache=true -f flannel-base.dockerfile -t registry.cn-hangzhou.aliyuncs.com/generals-kuber/flannel-base:1.0.0 .
FROM alpine

RUN apk add --no-cache iproute2 net-tools ca-certificates iptables strongswan && update-ca-certificates
RUN apk add wireguard-tools --no-cache --repository http://dl-cdn.alpinelinux.org/alpine/edge/testing
