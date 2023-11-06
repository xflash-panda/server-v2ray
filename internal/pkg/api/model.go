package api

import "github.com/xtls/xray-core/infra/conf"

// API is the interface for different panel's api.
type API interface {
	GetNodeInfo() (nodeInfo *NodeInfo, err error)
	GetUserList() (userList []*UserInfo, err error)
	ReportUserTraffic(userTraffic []*UserTraffic) (err error)
	Describe() *ClientInfo
	Debug()
}

type UserTraffic struct {
	UID      int   `json:"user_id"`
	Upload   int64 `json:"u"`
	Download int64 `json:"d"`
	Count    int64 `json:"n"`
}

type RepUserTraffic struct {
	Message string `json:"message"`
}

type NodeInfo struct {
	ID              int                   `json:"id"`
	ServerPort      int                   `json:"server_port"`
	TLS             int                   `json:"tls"`
	Network         string                `json:"network"`
	TlsConfig       *conf.TLSConfig       `json:"tls_settings"`
	WebSocketConfig *conf.WebSocketConfig `json:"ws_settings,omitempty"`
	H2Config        *conf.HTTPConfig      `json:"h2_config"`
	TcpConfig       *conf.TCPConfig       `json:"tcp_settings,omitempty"`
	GrpcConfig      *conf.GRPCConfig      `json:"grpc_settings,omitempty"`
	RouterSettings  *conf.RouterConfig    `json:"router_settings,omitempty"`
	DnsSettings     *conf.DNSConfig       `json:"dns_settings,omitempty"`
}

type RepNodeInfo struct {
	Data    *NodeInfo `json:"data"`
	Message string    `json:"message"`
}

type UserInfo struct {
	ID   int    `json:"id"`
	UUID string `json:"uuid"`
}

type RepUserList struct {
	Data    *[]UserInfo `json:"data"`
	Message string      `json:"message"`
}

type ClientInfo struct {
	APIHost string
	NodeID  int
	Token   string
}
