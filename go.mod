module github.com/ikedam/cloudbuild

go 1.13

// see https://github.com/moby/moby/issues/39302
// and use the oldest stable version 8840071c26093d0589edb659b329e82892e496c2,
// which contains https://github.com/moby/moby/pull/40021
// and not released yet.
replace github.com/docker/docker => github.com/docker/engine v1.4.2-0.20191124153605-8840071c2609

require (
	cloud.google.com/go v0.50.0
	cloud.google.com/go/storage v1.0.0
	github.com/Microsoft/hcsshim v0.8.7 // indirect
	github.com/docker/docker v0.0.0-00010101000000-000000000000
	github.com/golang/mock v1.3.1
	github.com/golang/protobuf v1.3.2
	github.com/googleapis/gax-go v2.0.2+incompatible // indirect
	github.com/mitchellh/go-homedir v1.1.0
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/rs/xid v1.2.1
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/cobra v0.0.5
	github.com/spf13/viper v1.6.1
	github.com/stretchr/testify v1.2.2
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/sys v0.0.0-20191228213918-04cbcbbfeed8 // indirect
	golang.org/x/xerrors v0.0.0-20191204190536-9bdfabe68543
	google.golang.org/api v0.15.0
	google.golang.org/genproto v0.0.0-20200108215221-bd8f9a0ef82f
	google.golang.org/grpc v1.21.1
	gopkg.in/yaml.v3 v3.0.0-20191120175047-4206685974f2
)
