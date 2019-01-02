# mtr_exporter
simple exporter for [mtr](http://www.bitwizard.nl/mtr/) stats for prometheus.

## Version v0.1.0 vs v0.2.0
* v0.1.0 runs the mtr command synchronously and provides the data as a 'snapshot' in time, thus you will need to configure the scrape timeout accordingly (scrape timeout >= cycles)
* v0.2.0 runs the mtr command asynchron in the background and provide the data in a more prometheus like way, also metrics were removed/added or renamed (see https://github.com/Shinzu/mtr_exporter/pull/1)

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

