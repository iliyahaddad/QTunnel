package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/qtunnel/qtunnel/internal/config"
	"github.com/qtunnel/qtunnel/internal/carrier/quic"
	"go.uber.org/zap"
)

var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func main() {
	var (
		configFile = flag.String("c", "", "Configuration file path")
		showVer    = flag.Bool("version", false, "Show version and exit")
		genSecret  = flag.Bool("secret", false, "Generate a random secret token")
		pingCmd    = flag.Bool("ping", false, "Test connectivity to server")
		certCmd    = flag.Bool("cert", false, "Show certificate info")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("qtunnel-diagnose version %s\n", version)
		fmt.Printf("Build time: %s\n", buildTime)
		fmt.Printf("Git commit: %s\n", gitCommit)
		os.Exit(0)
	}

	if *genSecret {
		token := make([]byte, 32)
		if _, err := rand.Read(token); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to generate secret: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(base64.StdEncoding.EncodeToString(token))
		os.Exit(0)
	}

	if *configFile == "" && !*pingCmd && !*certCmd {
		fmt.Fprintln(os.Stderr, "Error: config file required (-c)")
		flag.Usage()
		os.Exit(1)
	}

	if *pingCmd {
		if *configFile == "" {
			fmt.Fprintln(os.Stderr, "Error: config file required for ping")
			os.Exit(1)
		}
		if err := runPing(*configFile); err != nil {
			fmt.Fprintf(os.Stderr, "Ping failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Ping successful")
		os.Exit(0)
	}

	if *certCmd {
		if *configFile == "" {
			fmt.Fprintln(os.Stderr, "Error: config file required for cert")
			os.Exit(1)
		}
		if err := runCertInfo(*configFile); err != nil {
			fmt.Fprintf(os.Stderr, "Cert info failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Config check
	cfg, err := config.LoadClientConfig(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Configuration valid")
	fmt.Printf("Role: %s\n", cfg.Role)
	fmt.Printf("Server: %s\n", cfg.Server.Addr)
	fmt.Printf("Transport: %s\n", cfg.Transport.Type)
}

func runPing(configFile string) error {
	cfg, err := config.LoadClientConfig(configFile)
	if err != nil {
		return err
	}

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	dialer := quic.NewDialer(cfg.Transport.QUIC, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := dialer.Dial(ctx, cfg.Server.Addr, cfg.Server.SNI, cfg.TLS.InsecureSkipVerify)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}
	defer conn.Close()

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return fmt.Errorf("open stream failed: %w", err)
	}
	defer stream.Close()

	// Send a ping frame
	if _, err := stream.Write([]byte{0x06, 0x00, 0x00}); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	buf := make([]byte, 32)
	n, err := stream.Read(buf)
	if err != nil {
		return fmt.Errorf("read failed: %w", err)
	}

	fmt.Printf("Received %d bytes: %x\n", n, buf[:n])
	return nil
}

func runCertInfo(configFile string) error {
	cfg, err := config.LoadClientConfig(configFile)
	if err != nil {
		return err
	}

	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.Dial("tcp", cfg.Server.Addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         cfg.Server.SNI,
		InsecureSkipVerify: cfg.TLS.InsecureSkipVerify,
	})
	defer tlsConn.Close()

	if err := tlsConn.Handshake(); err != nil {
		return err
	}

	state := tlsConn.ConnectionState()
	fmt.Printf("Version: %x\n", state.Version)
	fmt.Printf("Cipher Suite: %x\n", state.CipherSuite)
	fmt.Printf("Server Name: %s\n", state.ServerName)
	fmt.Printf("ALPN: %s\n", state.NegotiatedProtocol)
	fmt.Printf("Peer Certificates: %d\n", len(state.PeerCertificates))
	for i, cert := range state.PeerCertificates {
		fmt.Printf("  Cert %d:\n", i)
		fmt.Printf("    Subject: %s\n", cert.Subject)
		fmt.Printf("    Issuer: %s\n", cert.Issuer)
		fmt.Printf("    NotBefore: %s\n", cert.NotBefore)
		fmt.Printf("    NotAfter: %s\n", cert.NotAfter)
		fmt.Printf("    DNSNames: %v\n", cert.DNSNames)
		fmt.Printf("    IPAddresses: %v\n", cert.IPAddresses)
	}
	return nil
}