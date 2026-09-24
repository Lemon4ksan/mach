package altsvc

import (
	"strconv"
	"strings"
	"time"
)

// Service represents an alternative service entry (RFC 7838).
type Service struct {
	ProtocolID string        // e.g., "h3", "h2"
	Host       string        // Alternative host (can be empty, meaning same host)
	Port       int           // Alternative port
	MaxAge     time.Duration // ma parameter, defaults to 24 hours (86400s)
	Persist    bool          // persist parameter
}

// Parse parses an Alt-Svc header value into a slice of services.
// It performs zero-allocation string slicing where possible.
// Example: h3=":443"; ma=2592000, h2="alt.example.com:443"; persist=1
func Parse(header string) []Service {
	if header == "clear" {
		return nil
	}

	var services []Service

	// Split by comma for multiple alternative services
	for part := range strings.SplitSeq(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		params := strings.Split(part, ";")
		if len(params) == 0 {
			continue
		}

		// Parse the protocol and host:port
		// Format: protocol-id="host:port"
		protocolID, hostPort, ok := strings.Cut(strings.TrimSpace(params[0]), "=")
		if !ok {
			continue
		}

		// Unquote the hostPort if needed
		if len(hostPort) >= 2 && hostPort[0] == '"' && hostPort[len(hostPort)-1] == '"' {
			hostPort = hostPort[1 : len(hostPort)-1]
		}

		// Parse host and port
		var host string
		var port int

		colonIdx := strings.LastIndexByte(hostPort, ':')
		if colonIdx != -1 {
			host = hostPort[:colonIdx]
			if p, err := strconv.Atoi(hostPort[colonIdx+1:]); err == nil {
				port = p
			} else {
				continue // invalid port
			}
		} else {
			continue // port is mandatory in Alt-Svc
		}

		svc := Service{
			ProtocolID: protocolID,
			Host:       host,
			Port:       port,
			MaxAge:     86400 * time.Second, // default per RFC
		}

		// Parse additional parameters
		for i := 1; i < len(params); i++ {
			param := strings.TrimSpace(params[i])
			if param == "" {
				continue
			}

			key, val, _ := strings.Cut(param, "=")

			switch key {
			case "ma":
				if secs, err := strconv.Atoi(val); err == nil {
					svc.MaxAge = time.Duration(secs) * time.Second
				}
			case "persist":
				if val == "1" {
					svc.Persist = true
				}
			}
		}

		services = append(services, svc)
	}

	return services
}
