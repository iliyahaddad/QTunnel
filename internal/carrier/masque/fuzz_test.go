package masque

import (
	"testing"
)

func FuzzURITemplateExpansion(f *testing.F) {
	testCases := []string{
		"https://example.com/.well-known/masque/udp/{target_host}/{target_port}/",
		"https://proxy.example.com/masque/udp/{target_host}/{target_port}",
		"https://{host}/masque/ip/{target}/{ipproto}/",
		"https://example.com/path/{param1}/{param2}/{param3}",
	}
	
	for _, tc := range testCases {
		f.Add(tc)
	}
	
	f.Fuzz(func(t *testing.T, template string) {
		// Test URI template expansion with various inputs
		c := &CONNECTUDPClient{}
		_, _ = c.expandURITemplate(template, "example.com", "443")
	})
}

func FuzzCapsuleParsing(f *testing.F) {
	// Valid capsule frames
	validCapsules := [][]byte{
		// DATAGRAM capsule (type 0x00) with 4 bytes data
		{0x00, 0x04, 0x01, 0x02, 0x03, 0x04},
		// ADDRESS_ASSIGN capsule (type 0x01) with IPv4
		{0x01, 0x08, 0x04, 0x0a, 0x00, 0x00, 0x01, 0x18},
		// ADDRESS_REQUEST capsule (type 0x02)
		{0x02, 0x05, 0x01, 0x04, 0x0a, 0x00, 0x00, 0x01, 0x18},
		// ROUTE_ADVERTISEMENT capsule (type 0x03)
		{0x03, 0x0a, 0x04, 0x0a, 0x00, 0x00, 0x00, 0x0a, 0x00, 0x00, 0x01, 0xff},
	}
	
	for _, c := range validCapsules {
		f.Add(c)
	}
	
	f.Fuzz(func(t *testing.T, data []byte) {
		client := &CONNECTIPClient{}
		client.parseCapsule(data)
	})
}

func FuzzURIParsing(f *testing.F) {
	uris := []string{
		"https://example.com:443/",
		"https://[::1]:443/",
		"https://example.com/.well-known/masque/udp/192.168.1.1/8080/",
		"https://example.com/.well-known/masque/ip/192.168.1.1/4/",
	}
	
	for _, u := range uris {
		f.Add(u)
	}
	
	f.Fuzz(func(t *testing.T, uri string) {
		c := &CONNECTUDPClient{}
		c.extractAuthority(uri)
	})
}

func parseCapsule(data []byte) (uint64, []byte, error) {
	if len(data) < 1 {
		return 0, nil, nil
	}
	
	// Simple varint parsing for fuzzing
	var capsuleType uint64
	n := 1
	if data[0] < 0x40 {
		capsuleType = uint64(data[0])
	} else if data[0] < 0x80 {
		if len(data) < 2 {
			return 0, nil, nil
		}
		capsuleType = uint64(data[0]&0x3f)<<8 | uint64(data[1])
		n = 2
	} else if data[0] < 0xc0 {
		if len(data) < 4 {
			return 0, nil, nil
		}
		capsuleType = uint64(data[0]&0x3f)<<24 | uint64(data[1])<<16 | uint64(data[2])<<8 | uint64(data[3])
		n = 4
	} else {
		if len(data) < 8 {
			return 0, nil, nil
		}
		capsuleType = uint64(data[0]&0x3f)<<56 | uint64(data[1])<<48 | uint64(data[2])<<40 | uint64(data[3])<<32 |
			uint64(data[4])<<24 | uint64(data[5])<<16 | uint64(data[6])<<8 | uint64(data[7])
		n = 8
	}
	
	if len(data) < n+1 {
		return capsuleType, nil, nil
	}
	
	// Parse length
	var length uint64
	m := 1
	if data[n] < 0x40 {
		length = uint64(data[n])
	} else if data[n] < 0x80 {
		if len(data) < n+2 {
			return capsuleType, nil, nil
		}
		length = uint64(data[n]&0x3f)<<8 | uint64(data[n+1])
		m = 2
	} else if data[n] < 0xc0 {
		if len(data) < n+4 {
			return capsuleType, nil, nil
		}
		length = uint64(data[n]&0x3f)<<24 | uint64(data[n+1])<<16 | uint64(data[n+2])<<8 | uint64(data[n+3])
		m = 4
	} else {
		if len(data) < n+8 {
			return capsuleType, nil, nil
		}
		length = uint64(data[n]&0x3f)<<56 | uint64(data[n+1])<<48 | uint64(data[n+2])<<40 | uint64(data[n+3])<<32 |
			uint64(data[n+4])<<24 | uint64(data[n+5])<<16 | uint64(data[n+6])<<8 | uint64(data[n+7])
		m = 8
	}
	
	if len(data) < n+m+int(length) {
		return capsuleType, nil, nil
	}
	
	capsuleData := data[n+m : n+m+int(length)]
	return capsuleType, capsuleData, nil
}