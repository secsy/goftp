// Copyright 2015 Muir Manders.  All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package goftp

import (
	"errors"
	"fmt"
	"net"
	"regexp"
)

// Create an FTP client using the default config. See DialConfig for
// information about "hosts".
func Dial(hosts ...string) (Conn, error) {
	return DialConfig(Config{}, hosts...)
}

// Create an FTP client using the given config. "hosts" is a list of IP
// addresses or hostnames with an optional port (defaults to 21).
// Hostnames will be expanded to all the IP addresses they resolve to. The
// client's connection pool will pick from all the addresses in a round-robin
// fashion.
func DialConfig(config Config, hosts ...string) (Conn, error) {
	expandedHosts, err := lookupHosts(hosts)
	if err != nil {
		return nil, err
	}

	return newClient(config, expandedHosts), nil
}

var hasPort = regexp.MustCompile(`^[^:]+:\d+$|\]:\d+$`)

func lookupHosts(hosts []string) ([]string, error) {
	if len(hosts) == 0 {
		return nil, errors.New("must specify at least one host")
	}

	var ret []string

	for i, host := range hosts {
		if !hasPort.MatchString(host) {
			host = fmt.Sprintf("[%s]:21", host)
		}
		hostnameOrIP, port, err := net.SplitHostPort(host)
		if err != nil {
			return nil, fmt.Errorf(`invalid host "%s"`, hosts[i])
		}

		if net.ParseIP(hostnameOrIP) != nil {
			// is IP, add to list
			ret = append(ret, host)
		} else {
			// not an IP, must be hostname
			ips, err := net.LookupHost(hostnameOrIP)

			// consider not returning error if other hosts in the list work
			if err != nil {
				return nil, fmt.Errorf(`error resolving host "%s": %s`, hostnameOrIP, err)
			}

			for _, ip := range ips {
				ret = append(ret, fmt.Sprintf("[%s]:%s", ip, port))
			}
		}
	}

	return ret, nil
}
