## docker build --no-cache=true -f flannel.dockerfile -t harbor.generals.space/kuber/flannel:1.0.1 .
FROM harbor.generals.space/kuber/flannel-base

COPY dist/flanneld /opt/bin/flanneld
COPY dist/mk-docker-opts.sh /opt/bin/

ENTRYPOINT ["/opt/bin/flanneld"]
