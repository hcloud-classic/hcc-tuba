package config

import (
	"github.com/Terry-Mao/goconf"
	"hcc/tuba/lib/logger"
)

var conf = goconf.New()
var config = tubaConfig{}
var err error

func parseGrpc() {
	config.GrpcConfig = conf.Get("grpc")
	if config.GrpcConfig == nil {
		logger.Logger.Panicln("no grpc section")
	}

	Grpc.Port, err = config.GrpcConfig.Int("port")
	if err != nil {
		logger.Logger.Panicln(err)
	}
}

// Init : Parse config file and initialize config structure
func Init() {
	if err = conf.Parse(configLocation); err != nil {
		logger.Logger.Panicln(err)
	}

	parseGrpc()
}
