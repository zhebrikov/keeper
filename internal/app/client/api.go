package client

import (
	gkclient "github.com/zhebrikov/gophkeeper/internal/client"
	"github.com/zhebrikov/gophkeeper/internal/config"
)

type gophkeeperClient = gkclient.API

var gophkeeperClientFactory = func(cfg config.ClientConfig) (*gophkeeperClient, error) {
	return gkclient.New(cfg)
}

func newGophkeeperClient(cfg config.ClientConfig) (*gophkeeperClient, error) {
	return gophkeeperClientFactory(cfg)
}
