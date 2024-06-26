package server

import (
	"context"
	"fmt"
	log "github.com/sirupsen/logrus"
	pb "github.com/xflash-panda/server-agent-proto/pkg"
	api "github.com/xflash-panda/server-client/pkg"
	_ "github.com/xflash-panda/server-vmess/internal/pkg/dep"
	"github.com/xflash-panda/server-vmess/internal/pkg/dispatcher"
	"github.com/xflash-panda/server-vmess/internal/pkg/service"
	"github.com/xtls/xray-core/app/dns"
	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/app/router"
	"github.com/xtls/xray-core/app/stats"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf"
	"sync"
	"time"
	"unsafe"
)

type Config struct {
	LogLevel  string
	AgentHost string
	AgentPort int
}

type Server struct {
	access        sync.Mutex
	service       service.Service
	config        *Config
	serviceConfig *service.Config
	Running       bool
}

func New(config *Config, serviceConfig *service.Config) *Server {
	return &Server{config: config, serviceConfig: serviceConfig}
}

func (s *Server) Start(agentClient pb.AgentClient) {
	s.access.Lock()
	defer s.access.Unlock()
	log.Infoln("server Start")
	ctx, cancel := context.WithTimeout(context.Background(), service.DefaultTimeout)
	defer cancel()

	r, err := agentClient.Config(ctx, &pb.ConfigRequest{Params: &pb.CommonParams{NodeId: int32(s.serviceConfig.NodeID), NodeType: pb.NodeType_VMESS}})
	if err != nil {
		panic(fmt.Errorf("get config eror: %v", err))
	}

	vmessConfig, err := api.UnmarshalVMessConfig(r.GetRawData())
	if err != nil {
		panic(err)
	}
	pbInBoundConfig, err := service.InboundBuilder(s.serviceConfig, vmessConfig)
	if err != nil {
		panic(fmt.Errorf("failed to build inbound config: %s", err))
	}

	pbOutBoundConfig, err := service.OutboundBuilder(vmessConfig)
	if err != nil {
		panic(fmt.Errorf("failed to build outbound config: %s", err))
	}

	var pbRouterConfig *router.Config
	if vmessConfig.RouterSettings != nil {
		pbRouterConfig, err = (*conf.RouterConfig)(unsafe.Pointer(vmessConfig.RouterSettings)).Build()
		if err != nil {
			panic(fmt.Errorf("failed to build router config:%s", err))
		}
	} else {
		routeConfig := &conf.RouterConfig{}
		pbRouterConfig, _ = routeConfig.Build()
	}

	var pbDnsConfig *dns.Config
	if vmessConfig.DnsSettings != nil {
		pbDnsConfig, err = (*conf.DNSConfig)(unsafe.Pointer(vmessConfig.DnsSettings)).Build()
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

	buildService := service.New(pbInBoundConfig.Tag, instance, s.serviceConfig, vmessConfig, agentClient)
	s.service = buildService
	if err := s.service.Start(); err != nil {
		panic(fmt.Errorf("failed to start build service: %s", err))
	}
	s.Running = true
	log.Infoln("server is running")
	log.Infoln("server is running")
	time.Sleep(1 * time.Minute)
	if err := s.service.StartMonitor(); err != nil {
		panic(fmt.Errorf("failed to start service monitor: %s", err))
	}
}

func (s *Server) loadCore(pbInboundConfig *core.InboundHandlerConfig, pbOutboundConfig *core.OutboundHandlerConfig,
	pbRouterConfig *router.Config, pbDnsConfig *dns.Config) (*core.Instance, error) {
	//Log Config
	logConfig := &conf.LogConfig{}
	logConfig.LogLevel = s.config.LogLevel
	if s.config.LogLevel != LogLevelDebug {
		logConfig.AccessLog = "none"
		logConfig.ErrorLog = "none"
		logConfig.DNSLog = false
	}
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
