package config

import "github.com/Terry-Mao/goconf"

var configLocation = "/etc/hcc/tuba/tuba.conf"

type tubaConfig struct {
	GrpcConfig *goconf.Section
}
