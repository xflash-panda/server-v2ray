// Package generate the InbounderConfig used by add inbound
package service

import (
	"encoding/json"
	"fmt"
	api "github.com/xflash-panda/server-client/pkg"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf"
	"unsafe"
)

// InboundBuilder build Inbound config for different protocol
func InboundBuilder(config *Config, nodeInfo *api.VMessConfig) (*core.InboundHandlerConfig, error) {
	inboundDetourConfig := &conf.InboundDetourConfig{}

	// Build Port
	portList := &conf.PortList{
		Range: []conf.PortRange{{From: uint32(nodeInfo.ServerPort), To: uint32(nodeInfo.ServerPort)}},
	}
	inboundDetourConfig.PortList = portList
	// Build Tag
	inboundDetourConfig.Tag = fmt.Sprintf("%s_%d", protocol, nodeInfo.ServerPort)
	// SniffingConfig
	sniffingConfig := &conf.SniffingConfig{
		Enabled:      true,
		DestOverride: &conf.StringList{"http", "tls"},
	}
	inboundDetourConfig.SniffingConfig = sniffingConfig

	var (
		streamSetting *conf.StreamConfig
		setting       json.RawMessage
	)

	var proxySetting interface{}
	// Build Protocol and Protocol setting
	proxySetting = &conf.VMessInboundConfig{}

	setting, err := json.Marshal(proxySetting)
	if err != nil {
		return nil, fmt.Errorf("marshal proxy %s config fialed: %s", protocol, err)
	}

	// Build streamSettings
	streamSetting = new(conf.StreamConfig)
	transportProtocol := conf.TransportProtocol(nodeInfo.Network)
	networkType, err := transportProtocol.Build()
	if err != nil {
		return nil, fmt.Errorf("convert TransportProtocol failed: %s", err)
	}
	if networkType == TCP {
		if nodeInfo.TcpConfig != nil {
			streamSetting.TCPSettings = (*conf.TCPConfig)(nodeInfo.TcpConfig)
		} else {
			streamSetting.TCPSettings = &conf.TCPConfig{}
		}
	} else if networkType == WS {
		if nodeInfo.WebSocketConfig != nil {
			streamSetting.WSSettings = (*conf.WebSocketConfig)(nodeInfo.WebSocketConfig)
		} else {
			streamSetting.WSSettings = &conf.WebSocketConfig{}
		}
	} else if networkType == GRPC {
		if nodeInfo.GrpcConfig != nil {
			streamSetting.GRPCConfig = (*conf.GRPCConfig)(nodeInfo.GrpcConfig)
		} else {
			streamSetting.GRPCConfig = &conf.GRPCConfig{}
		}
	} else if networkType == H2 {
		if nodeInfo.H2Config != nil {
			streamSetting.HTTPSettings = (*conf.HTTPConfig)(unsafe.Pointer(nodeInfo.H2Config))
		} else {
			streamSetting.HTTPSettings = &conf.HTTPConfig{}
		}
	}

	streamSetting.Network = &transportProtocol
	// Build TLS
	if nodeInfo.TLS > 0 {
		streamSetting.Security = TLS
		certFile, keyFile, err := getCertFile(config.Cert)
		if err != nil {
			return nil, err
		}

		var tlsSettings *conf.TLSConfig
		if nodeInfo.TlsConfig == nil {
			tlsSettings = &conf.TLSConfig{}
		} else {
			tlsSettings = (*conf.TLSConfig)(unsafe.Pointer(nodeInfo.TlsConfig))
		}

		tlsSettings.Certs = append(tlsSettings.Certs, &conf.TLSCertConfig{CertFile: certFile, KeyFile: keyFile, OcspStapling: 3600})
		streamSetting.TLSSettings = tlsSettings
	}

	inboundDetourConfig.Protocol = protocol
	inboundDetourConfig.StreamSetting = streamSetting
	inboundDetourConfig.Settings = &setting
	return inboundDetourConfig.Build()
}

// getCertFile
func getCertFile(certConfig *CertConfig) (certFile string, keyFile string, err error) {
	if certConfig.CertFile == "" || certConfig.KeyFile == "" {
		return "", "", fmt.Errorf("cert file path or key file path not exist")
	}
	return certConfig.CertFile, certConfig.KeyFile, nil
}
