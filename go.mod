module github.com/ikedam/cloudbuild

go 1.13

// see https://github.com/moby/moby/issues/39302
// and use the oldest stable version 8840071c26093d0589edb659b329e82892e496c2,
// which contains https://github.com/moby/moby/pull/40021
// and not released yet.
replace github.com/docker/docker => github.com/docker/engine v1.4.2-0.20191124153605-8840071c2609

require (
	github.com/Microsoft/hcsshim v0.8.7 // indirect
	github.com/docker/docker v0.0.0-00010101000000-000000000000
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/sirupsen/logrus v1.4.2 // indirect
	golang.org/x/sys v0.0.0-20191228213918-04cbcbbfeed8 // indirect
)
