package server

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/xflash-panda/server-vmess/internal/pkg/api"
	_ "github.com/xflash-panda/server-vmess/internal/pkg/dep"
	"github.com/xflash-panda/server-vmess/internal/pkg/service"
	"github.com/xtls/xray-core/app/dispatcher"
	"github.com/xtls/xray-core/app/dns"
	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/app/router"
	"github.com/xtls/xray-core/app/stats"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf"
	"sync"
)

type Config struct {
	LogLevel string
}

type Server struct {
	access        sync.Mutex
	instance      *core.Instance
	service       service.Service
	config        *Config
	apiConfig     *api.Config
	serviceConfig *service.Config
	Running       bool
}

func New(config *Config, apiConfig *api.Config, serviceConfig *service.Config) *Server {
	return &Server{config: config, apiConfig: apiConfig, serviceConfig: serviceConfig}
}

func (s *Server) Start() {
	s.access.Lock()
	defer s.access.Unlock()
	log.Infoln("server Start")
	apiClient := api.New(s.apiConfig)
	nodeInfo, err := apiClient.GetNodeInfo()
	if err != nil {
		panic(fmt.Errorf("failed to get node inf :%s", err))
	}

	pbInBoundConfig, err := service.InboundBuilder(s.serviceConfig, nodeInfo)
	if err != nil {
		panic(fmt.Errorf("failed to build inbound config: %s", err))
	}

	pbOutBoundConfig, err := service.OutboundBuilder(nodeInfo)
	if err != nil {
		panic(fmt.Errorf("failed to build outbound config: %s", err))
	}

	var pbRouterConfig *router.Config
	if nodeInfo.RouterSettings != nil {
		pbRouterConfig, err = nodeInfo.RouterSettings.Build()
		if err != nil {
			panic(fmt.Errorf("failed to build router config:%s", err))
		}
	} else {
		routeConfig := &conf.RouterConfig{}
		pbRouterConfig, _ = routeConfig.Build()
	}

	var pbDnsConfig *dns.Config
	if nodeInfo.DnsSettings != nil {
		pbDnsConfig, err = nodeInfo.DnsSettings.Build()
		if err != nil {
			panic(fmt.Errorf("failed to build dns condig:%s", err))
		}
	} else {
		coreDnsConfig := &conf.DNSConfig{}
		pbDnsConfig, _ = coreDnsConfig.Build()
	}

	instance, err := s.loadCore(pbInBoundConfig, pbOutBoundConfig, pbRouterConfig, pbDnsConfig)
	if err != nil {
		panic(err)
	}

	if err := instance.Start(); err != nil {
		panic(fmt.Errorf("failed to start instance: %s", err))
	}

	buildService := service.New(pbInBoundConfig.Tag, instance, s.serviceConfig, nodeInfo,
		apiClient.GetUserList, apiClient.ReportUserTraffic)
	s.service = buildService
	if err := s.service.Start(); err != nil {
		panic(fmt.Errorf("failed to start build service: %s", err))
	}
	s.Running = true
	log.Infoln("server is running")
}

func (s *Server) loadCore(pbInboundConfig *core.InboundHandlerConfig, pbOutboundConfig *core.OutboundHandlerConfig,
	pbRouterConfig *router.Config, pbDnsConfig *dns.Config) (*core.Instance, error) {
	//Log Config
	logConfig := &conf.LogConfig{}
	logConfig.LogLevel = s.config.LogLevel
	pbLogConfig := logConfig.Build()

	//InboundConfig
	inboundConfigs := make([]*core.InboundHandlerConfig, 1)
	inboundConfigs[0] = pbInboundConfig

	//OutBound config
	outBoundConfigs := make([]*core.OutboundHandlerConfig, 2)
	blockOutboundConfig, _ := service.OutboundBlockBuilder()
	outBoundConfigs[0] = pbOutboundConfig
	outBoundConfigs[1] = blockOutboundConfig

	//PolicyConfig
	policyConfig := &conf.PolicyConfig{}
	pbPolicy := &conf.Policy{
		StatsUserUplink:   true,
		StatsUserDownlink: true,
		Handshake:         &defaultConnectionConfig.Handshake,
		ConnectionIdle:    &defaultConnectionConfig.ConnIdle,
		UplinkOnly:        &defaultConnectionConfig.UplinkOnly,
		DownlinkOnly:      &defaultConnectionConfig.DownlinkOnly,
		BufferSize:        &defaultConnectionConfig.BufferSize,
	}
	policyConfig.Levels = map[uint32]*conf.Policy{0: pbPolicy}
	pbPolicyConfig, _ := policyConfig.Build()
	pbCoreConfig := &core.Config{
		App: []*serial.TypedMessage{
			serial.ToTypedMessage(pbLogConfig),
			serial.ToTypedMessage(pbPolicyConfig),
			serial.ToTypedMessage(pbDnsConfig),
			serial.ToTypedMessage(&stats.Config{}),
			serial.ToTypedMessage(&dispatcher.Config{}),
			serial.ToTypedMessage(&proxyman.InboundConfig{}),
			serial.ToTypedMessage(&proxyman.OutboundConfig{}),
			serial.ToTypedMessage(pbRouterConfig),
		},
		Outbound: outBoundConfigs,
		Inbound:  inboundConfigs,
	}
	instance, err := core.New(pbCoreConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create instance: %s", err)
	}
	return instance, nil
}

func (s *Server) Close() {
	s.access.Lock()
	defer s.access.Unlock()
	err := s.service.Close()
	if err != nil {
		log.Panicf("server Close fialed: %s", err)
	}
	log.Infoln("server close")
}
