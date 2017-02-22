# mtr_exporter
simple exporter for [mtr](http://www.bitwizard.nl/mtr/) stats for prometheus.

## Getting Started

To run it:

```bash
./mtr_exporter [flags]
```

Help on flags:

```bash
./mtr_exporter --help
```


## Usage

In order to work you need to install the mtr package for your dirstribution eg Ubuntu:

```bash
sudo apt-get install mtr-tiny
```

In the config file you can define mtr arguments you want to use and the hosts you want to trace against.

Then simply run the exporter with the config file. This file can be in the same directory(standard location with name mtr.yaml) or somewhere else in the filesystem.

```bash
./mtr_exporter -config.file mtr.yaml
```

if you want to run it as non root under linux you must add the cap_net_raw capability for the mtr binary

```bash
sudo setcap cap_net_raw+ep /usr/bin/mtr
```

### Building

```bash
make build
```

