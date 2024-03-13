package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	api "github.com/xflash-panda/server-client/pkg"
	"github.com/xflash-panda/server-vmess/internal/app/server"
	"github.com/xflash-panda/server-vmess/internal/pkg/service"
	"github.com/xtls/xray-core/core"
	"io/ioutil"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

const (
	Name      = "vmess-node"
	Version   = "0.1.19"
	CopyRight = "XFLASH-PANDA@2021"
)

func init() {
	cli.VersionFlag = &cli.BoolFlag{
		Name:    "version",
		Aliases: []string{"V"},
		Usage:   "print only the version",
	}
	cli.ErrWriter = ioutil.Discard

	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("version=%s xray.version=%s\n", Version, core.Version())
	}
}

func main() {
	var config server.Config
	var apiConfig api.Config
	var serviceConfig service.Config
	var certConfig service.CertConfig

	app := &cli.App{
		Name:      Name,
		Version:   Version,
		Copyright: CopyRight,
		Usage:     "Provide vmess service for the v2Board(XFLASH-PANDA)",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "api",
				Usage:       "Server address",
				EnvVars:     []string{"X_PANDA_VMESS_API", "API"},
				Required:    true,
				Destination: &apiConfig.APIHost,
			},
			&cli.StringFlag{
				Name:        "token",
				Usage:       "Token of server API",
				EnvVars:     []string{"X_PANDA_VMESS_TOKEN", "TOKEN"},
				Required:    true,
				Destination: &apiConfig.Token,
			},

			&cli.StringFlag{
				Name:        "cert_file",
				Usage:       "Cert file",
				EnvVars:     []string{"X_PANDA_VMESS_CERT_FILE", "CERT_FILE"},
				Value:       "/root/.cert/server.crt",
				Required:    false,
				DefaultText: "/root/.cert/server.crt",
				Destination: &certConfig.CertFile,
			},
			&cli.StringFlag{
				Name:        "key_file",
				Usage:       "Key file",
				EnvVars:     []string{"X_PANDA_VMESS_KEY_FILE", "KEY_FILE"},
				Value:       "/root/.cert/server.key",
				Required:    false,
				DefaultText: "/root/.cert/server.key",
				Destination: &certConfig.KeyFile,
			},
			&cli.IntFlag{
				Name:        "node",
				Usage:       "Node ID",
				EnvVars:     []string{"X_PANDA_VMESS_NODE", "NODE"},
				Required:    true,
				Destination: &serviceConfig.NodeID,
			},
			&cli.DurationFlag{
				Name:        "fetch_users_interval, fui",
				Usage:       "API request cycle(fetch users), unit: second",
				EnvVars:     []string{"X_PANDA_VMESS_FETCH_USER_INTERVAL", "FETCH_USER_INTERVAL"},
				Value:       time.Second * 60,
				DefaultText: "60",
				Required:    false,
				Destination: &serviceConfig.FetchUsersInterval,
			},
			&cli.DurationFlag{
				Name:        "report_traffics_interval, fui",
				Usage:       "API request cycle(report traffics), unit: second",
				EnvVars:     []string{"X_PANDA_VMESS_FETCH_USER_INTERVAL", "REPORT_TRAFFICS_INTERVAL"},
				Value:       time.Second * 80,
				DefaultText: "80",
				Required:    false,
				Destination: &serviceConfig.ReportTrafficsInterval,
			},
			&cli.StringFlag{
				Name:        "log_mode",
				Value:       server.LogLevelError,
				Usage:       "Log mode",
				EnvVars:     []string{"X_PANDA_VMESS_LOG_LEVEL", "LOG_LEVEL"},
				Destination: &config.LogLevel,
				Required:    false,
			},
		},
		Before: func(c *cli.Context) error {
			log.SetFormatter(&log.TextFormatter{})
			if config.LogLevel == server.LogLevelDebug {
				log.SetFormatter(&log.TextFormatter{
					FullTimestamp: true,
				})
				log.SetLevel(log.DebugLevel)
				log.SetReportCaller(true)
			} else if config.LogLevel == server.LogLevelInfo {
				log.SetLevel(log.InfoLevel)
			} else if config.LogLevel == server.LogLevelError {
				log.SetLevel(log.ErrorLevel)
			} else {
				return fmt.Errorf("log mode %s not supported", config.LogLevel)
			}
			return nil
		},
		Action: func(c *cli.Context) error {
			if config.LogLevel != server.LogLevelDebug {
				defer func() {
					if r := recover(); r != nil {
						log.Fatal(r)
					}
				}()
			}
			serviceConfig.Cert = &certConfig
			serv := server.New(&config, &apiConfig, &serviceConfig)
			serv.Start()
			defer serv.Close()
			runtime.GC()
			{
				osSignals := make(chan os.Signal, 1)
				signal.Notify(osSignals, os.Interrupt, os.Kill, syscall.SIGTERM)
				<-osSignals
			}
			return nil
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
