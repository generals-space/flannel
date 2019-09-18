## docker build --no-cache=true -f flannel-base.dockerfile -t harbor.generals.space/kuber/flannel-base:1.0.0 .
FROM alpine

RUN apk add --no-cache iproute2 net-tools ca-certificates iptables strongswan && update-ca-certificates
RUN apk add wireguard-tools --no-cache --repository http://dl-cdn.alpinelinux.org/alpine/edge/testing

