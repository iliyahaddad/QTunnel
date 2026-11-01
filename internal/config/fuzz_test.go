package config

import (
	"testing"
)

func FuzzConfigParse(f *testing.F) {
	validConfigs := []string{
		`role: "client"
server:
  addr: "example.com:4433"
  sni: "example.com"
auth:
  token: "dGVzdC10b2tlbi10aGF0LWlzLTMtMTEteHh4eHh4eHg="
transport:
  type: "quic"
proxy:
  mode: "tcp"
  listen: "127.0.0.1:8080"
limits:
  max_connections: 1000
  max_streams_per_conn: 100
  stream_buffer_size: "1MB"
  conn_buffer_size: "10MB"
  max_frame_size: "16KB"
logging:
  level: "info"
  format: "console"
  output: "stderr"`,
		`role: "server"
listen:
  addr: ":4433"
tls:
  cert_file: "server.crt"
  key_file: "server.key"
auth:
  tokens: ["token1"]
transport:
  type: "quic"
proxy:
  mode: "tcp"
  allowlist: ["*"]
  denylist: ["10.0.0.0/8"]
limits:
  max_connections: 1000
  max_streams_per_conn: 100
logging:
  level: "info"`,
	}
	
	for _, cfg := range validConfigs {
		f.Add([]byte(cfg))
	}
	
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = LoadClientConfig(string(data))
		_, _ = LoadServerConfig(string(data))
	})
}