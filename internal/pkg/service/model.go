package service

import "time"

const (
	protocol       = "vmess"
	TLS            = "tls"
	TCP            = "tcp"
	WS             = "ws"
	GRPC           = "grpc"
	H2             = "h2"
	DefaultTimeout = 15 * time.Second
)

// Service is the interface of all the services running in the panel
type Service interface {
	Start() error
	Close() error
	StartMonitor() error
}

type CertConfig struct {
	LogLevel string
	CertFile string
	KeyFile  string
}
