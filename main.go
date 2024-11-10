package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	_ "github.com/v2fly/v2ray-core/v5/app/proxyman/inbound"
	_ "github.com/v2fly/v2ray-core/v5/app/proxyman/outbound"

	core "github.com/v2fly/v2ray-core/v5"
	vlog "github.com/v2fly/v2ray-core/v5/app/log"
	clog "github.com/v2fly/v2ray-core/v5/common/log"

	"github.com/v2fly/v2ray-core/v5/app/dispatcher"
	"github.com/v2fly/v2ray-core/v5/app/proxyman"
	"github.com/v2fly/v2ray-core/v5/common/net"
	"github.com/v2fly/v2ray-core/v5/common/platform/filesystem"
	"github.com/v2fly/v2ray-core/v5/common/protocol"
	"github.com/v2fly/v2ray-core/v5/common/serial"
	"github.com/v2fly/v2ray-core/v5/proxy/dokodemo"
	"github.com/v2fly/v2ray-core/v5/proxy/freedom"
	"github.com/v2fly/v2ray-core/v5/transport/internet"
	"github.com/v2fly/v2ray-core/v5/transport/internet/grpc"
	"github.com/v2fly/v2ray-core/v5/transport/internet/quic"
	"github.com/v2fly/v2ray-core/v5/transport/internet/tls"
	"github.com/v2fly/v2ray-core/v5/transport/internet/tls/utls"
	"github.com/v2fly/v2ray-core/v5/transport/internet/websocket"
)

var (
	VERSION = "custom"
)

var (
	vpn          = flag.Bool("V", false, "Run in VPN mode.")
	fastOpen     = flag.Bool("fast-open", false, "Enable TCP fast open.")
	localAddr    = flag.String("localAddr", "127.0.0.1", "local address to listen on.")
	localPort    = flag.String("localPort", "1984", "local port to listen on.")
	remoteAddr   = flag.String("remoteAddr", "127.0.0.1", "remote address to forward.")
	remotePort   = flag.String("remotePort", "1080", "remote port to forward.")
	path         = flag.String("path", "/", "URL path for websocket.")
	serviceName  = flag.String("serviceName", "GunService", "Service name for grpc.")
	host         = flag.String("host", "cloudfront.com", "Hostname for server.")
	tlsEnabled   = flag.Bool("tls", false, "Enable TLS.")
	cert         = flag.String("cert", "", "Path to TLS certificate file. Overrides certRaw. Default: ~/.acme.sh/{host}/fullchain.cer")
	certRaw      = flag.String("certRaw", "", "Raw TLS certificate content. Intended only for Android.")
	key          = flag.String("key", "", "(server) Path to TLS key file. Default: ~/.acme.sh/{host}/{host}.key")
	mode         = flag.String("mode", "websocket", "Transport mode: websocket, quic (enforced tls), grpc.")
	mux          = flag.Int("mux", 1, "Concurrent multiplexed connections (websocket client mode only).")
	server       = flag.Bool("server", false, "Run in server mode")
	logLevel     = flag.String("loglevel", "", "loglevel for v2ray: debug, info, warning (default), error, none.")
	version      = flag.Bool("version", false, "Show current version of v2ray-plugin")
	fwmark       = flag.Int("fwmark", 0, "Set SO_MARK option for outbound sockets.")
	insecure     = flag.Bool("insecure", false, "Allow insecure certificate from server, commonly self signed.")
	pinnedsha256 = flag.String("pinnedSha256", "", "Pinned Certificate chain sha256 fingerprint. Seprate with #.")
	useragent    = flag.String("userAgent", "", "User agent(base64) to include in http request.")
	bufsize      = flag.Int("bufSize", 0, "Set snd/recv socket buffer size")
)

func homeDir() string {
	usr, err := user.Current()
	if err != nil {
		logFatal(err)
		os.Exit(1)
	}
	return usr.HomeDir
}

func readCertificate() ([]byte, error) {
	if *cert != "" {
		return filesystem.ReadFile(*cert)
	}
	if *certRaw != "" {
		certHead := "-----BEGIN CERTIFICATE-----"
		certTail := "-----END CERTIFICATE-----"
		fixedCert := certHead + "\n" + *certRaw + "\n" + certTail
		return []byte(fixedCert), nil
	}
	panic("thou shalt not reach hear")
}

func logConfig(logLevel string) *vlog.Config {
	config := &vlog.Config{
		Error:  &vlog.LogSpecification{Type: vlog.LogType_Console, Level: clog.Severity_Warning},
		Access: &vlog.LogSpecification{Type: vlog.LogType_Console},
	}
	level := strings.ToLower(logLevel)
	switch level {
	case "debug":
		config.Error.Level = clog.Severity_Debug
	case "info":
		config.Error.Level = clog.Severity_Info
	case "error":
		config.Error.Level = clog.Severity_Error
	case "none":
		config.Error.Type = vlog.LogType_None
		config.Access.Type = vlog.LogType_None
	}
	return config
}

func parseLocalAddr(localAddr string) []string {
	return strings.Split(localAddr, "|")
}

func generateConfig() (*core.Config, error) {
	lport, err := net.PortFromString(*localPort)
	if err != nil {
		return nil, newError("invalid localPort:", *localPort).Base(err)
	}
	rport, err := strconv.ParseUint(*remotePort, 10, 32)
	if err != nil {
		return nil, newError("invalid remotePort:", *remotePort).Base(err)
	}
	outboundProxy := serial.ToTypedMessage(&freedom.Config{
		DestinationOverride: &freedom.DestinationOverride{
			Server: &protocol.ServerEndpoint{
				Address: net.NewIPOrDomain(net.ParseAddress(*remoteAddr)),
				Port:    uint32(rport),
			},
		},
	})

	var transportSettings proto.Message
	var connectionReuse bool
	switch *mode {
	case "websocket":
		if *server {
			transportSettings = &websocket.Config{
				Path: *path,
				Header: []*websocket.Header{
					{Key: "Host", Value: *host},
					{Key: "content-type", Value: "application/vnd.ms-cab-compressed"},
					{Key: "server", Value: "ECAcc (lac/55D2)"},
					{Key: "etag", Value: "80b93e24a2b0d71:0"},
				},
			}
		} else {
			var ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36 Edg/129.0.0.0"
			if *useragent != "" {
				u, err := base64.StdEncoding.DecodeString(*useragent)
				if err != nil {
					logWarn("Can not decode provided useragent !")
				} else {
					ua = string(u)
				}
			}

			transportSettings = &websocket.Config{
				Path: *path,
				Header: []*websocket.Header{
					{Key: "Host", Value: *host},
					{Key: "User-Agent", Value: ua},
				},
			}
		}
		if *mux != 0 {
			connectionReuse = true
		}
	case "quic":
		transportSettings = &quic.Config{
			Security: &protocol.SecurityConfig{Type: protocol.SecurityType_NONE},
		}
		*tlsEnabled = true
	case "grpc":
		transportSettings = &grpc.Config{
			ServiceName: *serviceName,
		}
	default:
		return nil, newError("unsupported mode:", *mode)
	}

	// hack v2ray-core grpc protocolName
	if *mode == "grpc" {
		*mode = "gun"
	}

	streamConfig := internet.StreamConfig{
		ProtocolName: *mode,
		TransportSettings: []*internet.TransportConfig{{
			ProtocolName: *mode,
			Settings:     serial.ToTypedMessage(transportSettings),
		}},
	}

	var socketConfig *internet.SocketConfig
	if *fastOpen || *fwmark != 0 {
		socketConfig = &internet.SocketConfig{}
		if *fastOpen {
			socketConfig.Tfo = internet.SocketConfig_Enable
		}
		if *fwmark != 0 {
			socketConfig.Mark = uint32(*fwmark)
		}
	}
	if runtime.GOOS == "windows" {
		if !(*bufsize > 0) {
		// set a higher value than default
			*bufsize = 196608
		}
		// 64k is typically default. so just set the result to 0 to use system default 
		if *bufsize == 65536 {
			*bufsize = 0
		}
	}

	if *bufsize > 0 {
		if socketConfig != nil {
			socketConfig.TxBufSize = int64(*bufsize)
			socketConfig.RxBufSize = int64(*bufsize)
		} else {
			socketConfig = &internet.SocketConfig{
				TxBufSize: int64(*bufsize),
				RxBufSize: int64(*bufsize),
			}
		}
	}
	if socketConfig != nil {
		streamConfig.SocketSettings = socketConfig
	}
	if *tlsEnabled {
		tlsConfig := tls.Config{ServerName: *host}
		if *server {
			certificate := tls.Certificate{}
			if *cert == "" && *certRaw == "" {
				*cert = fmt.Sprintf("%s/.acme.sh/%s/fullchain.cer", homeDir(), *host)
				logWarn("No TLS cert specified, trying", *cert)
			}
			certificate.Certificate, err = readCertificate()
			if err != nil {
				return nil, newError("failed to read cert").Base(err)
			}
			if *key == "" {
				*key = fmt.Sprintf("%[1]s/.acme.sh/%[2]s/%[2]s.key", homeDir(), *host)
				logWarn("No TLS key specified, trying", *key)
			}
			certificate.Key, err = filesystem.ReadFile(*key)
			if err != nil {
				return nil, newError("failed to read key file").Base(err)
			}
			tlsConfig.Certificate = []*tls.Certificate{&certificate}
			streamConfig.SecurityType = serial.GetMessageType(&tlsConfig)
			streamConfig.SecuritySettings = []*anypb.Any{serial.ToTypedMessage(&tlsConfig)}
		} else {
			if *cert != "" || *certRaw != "" {
				certificate := tls.Certificate{Usage: tls.Certificate_AUTHORITY_VERIFY}
				certificate.Certificate, err = readCertificate()
				if err != nil {
					return nil, newError("failed to read cert").Base(err)
				}
				tlsConfig.Certificate = []*tls.Certificate{&certificate}
			}
			if *insecure {
				tlsConfig.AllowInsecure = true
				tlsConfig.AllowInsecureIfPinnedPeerCertificate = true
			}
			if *pinnedsha256 != "" {
				fp := strings.Split(*pinnedsha256, "#")
				var fps = make([][]byte, 3)
				for i := range fp {
					if i >= 3 {
						fps = append(fps, []byte(fp[i]))
					} else {
						fps[i] = []byte(fp[i])
					}
				}
				tlsConfig.PinnedPeerCertificateChainSha256 = fps
			}
			utlsConfig := utls.Config{Imitate: "randomized"}
			utlsConfig.TlsConfig = &tlsConfig
			streamConfig.SecurityType = serial.GetMessageType(&utlsConfig)
			streamConfig.SecuritySettings = []*anypb.Any{serial.ToTypedMessage(&utlsConfig)}
		}
	}

	apps := []*anypb.Any{
		serial.ToTypedMessage(&dispatcher.Config{}),
		serial.ToTypedMessage(&proxyman.InboundConfig{}),
		serial.ToTypedMessage(&proxyman.OutboundConfig{}),
		serial.ToTypedMessage(logConfig(*logLevel)),
	}

	if *server {
		proxyAddress := net.LocalHostIP
		if connectionReuse {
			// This address is required when mux is used on client.
			// dokodemo is not aware of mux connections by itself.
			proxyAddress = net.ParseAddress("v1.mux.cool")
		}
		localAddrs := parseLocalAddr(*localAddr)
		inbounds := make([]*core.InboundHandlerConfig, len(localAddrs))

		for i := 0; i < len(localAddrs); i++ {
			inbounds[i] = &core.InboundHandlerConfig{
				ReceiverSettings: serial.ToTypedMessage(&proxyman.ReceiverConfig{
					PortRange:      net.SinglePortRange(lport),
					Listen:         net.NewIPOrDomain(net.ParseAddress(localAddrs[i])),
					StreamSettings: &streamConfig,
				}),
				ProxySettings: serial.ToTypedMessage(&dokodemo.Config{
					Address:  net.NewIPOrDomain(proxyAddress),
					Networks: []net.Network{net.Network_TCP},
				}),
			}
		}

		return &core.Config{
			Inbound: inbounds,
			Outbound: []*core.OutboundHandlerConfig{{
				ProxySettings: outboundProxy,
			}},
			App: apps,
		}, nil
	} else {
		senderConfig := proxyman.SenderConfig{StreamSettings: &streamConfig}
		if connectionReuse {
			senderConfig.MultiplexSettings = &proxyman.MultiplexingConfig{Enabled: true, Concurrency: uint32(*mux)}
		}
		return &core.Config{
			Inbound: []*core.InboundHandlerConfig{{
				ReceiverSettings: serial.ToTypedMessage(&proxyman.ReceiverConfig{
					PortRange: net.SinglePortRange(lport),
					Listen:    net.NewIPOrDomain(net.ParseAddress(*localAddr)),
				}),
				ProxySettings: serial.ToTypedMessage(&dokodemo.Config{
					Address:  net.NewIPOrDomain(net.LocalHostIP),
					Networks: []net.Network{net.Network_TCP},
				}),
			}},
			Outbound: []*core.OutboundHandlerConfig{{
				SenderSettings: serial.ToTypedMessage(&senderConfig),
				ProxySettings:  outboundProxy,
			}},
			App: apps,
		}, nil
	}
}

func startV2Ray() (core.Server, error) {

	opts, err := parseEnv()

	if err == nil {
		if c, b := opts.Get("mode"); b {
			*mode = c
		}
		if c, b := opts.Get("mux"); b {
			if i, err := strconv.Atoi(c); err == nil {
				*mux = i
			} else {
				logWarn("failed to parse mux, use default value")
			}
		}
		if _, b := opts.Get("tls"); b {
			*tlsEnabled = true
		}
		if c, b := opts.Get("host"); b {
			*host = c
		}
		if c, b := opts.Get("path"); b {
			*path = c
		}
		if c, b := opts.Get("serviceName"); b {
			*serviceName = c
		}
		if c, b := opts.Get("cert"); b {
			*cert = c
		}
		if c, b := opts.Get("certRaw"); b {
			*certRaw = c
		}
		if c, b := opts.Get("key"); b {
			*key = c
		}
		if c, b := opts.Get("loglevel"); b {
			*logLevel = c
		}
		if _, b := opts.Get("server"); b {
			*server = true
		}
		if c, b := opts.Get("localAddr"); b {
			if *server {
				*remoteAddr = c
			} else {
				*localAddr = c
			}
		}
		if c, b := opts.Get("localPort"); b {
			if *server {
				*remotePort = c
			} else {
				*localPort = c
			}
		}
		if c, b := opts.Get("remoteAddr"); b {
			if *server {
				*localAddr = c
			} else {
				*remoteAddr = c
			}
		}
		if c, b := opts.Get("remotePort"); b {
			if *server {
				*localPort = c
			} else {
				*remotePort = c
			}
		}

		if _, b := opts.Get("fastOpen"); b {
			*fastOpen = true
		}

		if _, b := opts.Get("__android_vpn"); b {
			*vpn = true
		}

		if c, b := opts.Get("fwmark"); b {
			if i, err := strconv.Atoi(c); err == nil {
				*fwmark = i
			} else {
				logWarn("failed to parse fwmark, use default value")
			}
		}

		if _, b := opts.Get("insecure"); b {
			*insecure = true
		}
		if c, b := opts.Get("pinnedsha256"); b {
			*pinnedsha256 = c
		}

		if c, b := opts.Get("useragent"); b {
			if !*server {
				*useragent = c
			}
		}

		if c, b := opts.Get("bufSize"); b {
			if i, err := strconv.Atoi(c); err == nil {
				*bufsize = i
			} else {
				logWarn("failed to parse buffer size !")
			}
		}

		if *vpn {
			registerControlFunc()
		}
	}

	config, err := generateConfig()
	if err != nil {
		return nil, newError("failed to parse config").Base(err)
	}
	instance, err := core.New(config)
	if err != nil {
		return nil, newError("failed to create v2ray instance").Base(err)
	}
	return instance, nil
}

func printCoreVersion() {
	version := core.VersionStatement()
	for _, s := range version {
		logInfo(s)
	}
}

func printVersion() {
	fmt.Println("v2ray-plugin", VERSION)
	fmt.Println("Go version", runtime.Version())
	fmt.Println("Yet another SIP003 plugin for shadowsocks")
}

func main() {
	flag.Parse()

	if *version {
		printVersion()
		return
	}

	logInit()

	printCoreVersion()

	server, err := startV2Ray()
	if err != nil {
		logFatal(err.Error())
		// Configuration error. Exit with a special value to prevent systemd from restarting.
		os.Exit(23)
	}
	if err := server.Start(); err != nil {
		logFatal("failed to start server:", err.Error())
		os.Exit(1)
	}

	defer func() {
		err := server.Close()
		if err != nil {
			logWarn(err.Error())
		}
	}()

	{
		osSignals := make(chan os.Signal, 1)
		signal.Notify(osSignals, os.Interrupt, os.Kill, syscall.SIGTERM)
		<-osSignals
	}
}
