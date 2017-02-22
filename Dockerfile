FROM        quay.io/prometheus/busybox:latest
MAINTAINER  The Prometheus Authors <prometheus-developers@googlegroups.com>

COPY mtr_exporter /bin/mtr_exporter

ENTRYPOINT ["/bin/mtr_exporter"]
EXPOSE     9116
