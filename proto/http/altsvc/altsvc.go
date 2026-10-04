package altsvc

import (
	"iter"
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

// All returns an iterator over alternative services without allocating slices.
func All(header string) iter.Seq[Service] {
	return func(yield func(Service) bool) {
		if header == "clear" {
			return
		}

		for part := range strings.SplitSeq(header, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}

			var firstPart string
			var hasFirst bool
			var svc Service

			for param := range strings.SplitSeq(part, ";") {
				param = strings.TrimSpace(param)
				if param == "" {
					continue
				}

				if !hasFirst {
					firstPart = param
					hasFirst = true

					protocolID, hostPort, ok := strings.Cut(firstPart, "=")
					if !ok {
						break
					}

					if len(hostPort) >= 2 && hostPort[0] == '"' && hostPort[len(hostPort)-1] == '"' {
						hostPort = hostPort[1 : len(hostPort)-1]
					}

					colonIdx := strings.LastIndexByte(hostPort, ':')
					if colonIdx == -1 {
						break
					}

					p, err := strconv.Atoi(hostPort[colonIdx+1:])
					if err != nil {
						break
					}

					svc = Service{
						ProtocolID: protocolID,
						Host:       hostPort[:colonIdx],
						Port:       p,
						MaxAge:     86400 * time.Second,
					}
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

			if hasFirst && svc.Port != 0 {
				if !yield(svc) {
					return
				}
			}
		}
	}
}

// Parse parses an Alt-Svc header value into a slice of services.
func Parse(header string) []Service {
	var services []Service
	for svc := range All(header) {
		services = append(services, svc)
	}
	return services
}

