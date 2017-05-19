go-mtr
======

A go wrapper on the mtr process. This package uses `mtr --raw` and
parses the output. The results are slightly different than running
`mtr --report` directly because `mtr` itself keeps a running total
of the mean/standard deviation/interarrival jitter/etc. This package
calculates these values over the entire package sequence.

mtr is required to use this package.

Documentation can be found on [godoc](http://godoc.org/github.com/fastly/go-mtr).
