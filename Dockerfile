FROM        quay.io/prometheus/busybox:latest
MAINTAINER  The Prometheus Authors <prometheus-developers@googlegroups.com>

COPY mtr_exporter /bin/mtr_exporter
COPY mtr.yaml /etc/mtr.yaml

ENTRYPOINT ["/bin/mtr_exporter", "-config.file", "/etc/mtr.yaml"]
EXPOSE     9116
