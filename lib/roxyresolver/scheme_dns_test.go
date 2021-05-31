package roxyresolver

import (
	"net"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/chronos-tachyon/roxy/internal/constants"
)

func TestDNSTarget_RoundTrip(t *testing.T) {
	type testRow struct {
		Input   string
		RoxyIn  Target
		DNS     DNSTarget
		RoxyOut Target
		Output  string
	}

	testData := []testRow{
		{
			Input: "dns:example.com",
			RoxyIn: Target{
				Scheme:     constants.SchemeDNS,
				Endpoint:   "example.com",
				ServerName: "example.com",
				HasSlash:   false,
			},
			DNS: DNSTarget{
				Host:       "example.com",
				Port:       constants.PortHTTPS,
				ServerName: "example.com",
				Balancer:   RandomBalancer,
			},
			RoxyOut: Target{
				Scheme:     constants.SchemeDNS,
				Endpoint:   "example.com:443",
				Query:      url.Values{"balancer": []string{"random"}},
				ServerName: "example.com",
				HasSlash:   true,
			},
			Output: "dns:///example.com:443?balancer=random",
		},
		{
			Input: "dns://8.8.8.8/example.com:80?pollInterval=1m",
			RoxyIn: Target{
				Scheme:     constants.SchemeDNS,
				Authority:  "8.8.8.8",
				Endpoint:   "example.com:80",
				Query:      url.Values{"pollInterval": []string{"1m"}},
				ServerName: "example.com",
				HasSlash:   true,
			},
			DNS: DNSTarget{
				ResolverAddr: &net.TCPAddr{IP: net.IP{8, 8, 8, 8}, Port: 53},
				Host:         "example.com",
				Port:         constants.PortHTTP,
				ServerName:   "example.com",
				Balancer:     RandomBalancer,
				PollInterval: 1 * time.Minute,
			},
			RoxyOut: Target{
				Scheme:     constants.SchemeDNS,
				Authority:  "8.8.8.8:53",
				Endpoint:   "example.com:80",
				Query:      url.Values{"balancer": []string{"random"}, "pollInterval": []string{"1m0s"}},
				ServerName: "example.com",
				HasSlash:   true,
			},
			Output: "dns://8.8.8.8:53/example.com:80?balancer=random&pollInterval=1m0s",
		},
	}

	for _, row := range testData {
		t.Run(row.Input, func(t *testing.T) {
			var roxyIn Target
			if err := roxyIn.Parse(row.Input); err != nil {
				t.Errorf("Target.Parse failed: %v", err)
				return
			}
			if !reflect.DeepEqual(roxyIn, row.RoxyIn) {
				t.Errorf("Target.Parse returned %#v, expected %#v", roxyIn, row.RoxyIn)
			}

			var target DNSTarget
			if err := target.FromTarget(roxyIn, constants.PortHTTPS); err != nil {
				t.Errorf("DNSTarget.FromTarget failed: %v", err)
				return
			}
			if !reflect.DeepEqual(target, row.DNS) {
				t.Errorf("DNSTarget.FromTarget returned %#v, expected %#v", target, row.DNS)
			}

			roxyOut := target.AsTarget()
			if !reflect.DeepEqual(roxyOut, row.RoxyOut) {
				t.Errorf("DNSTarget.AsTarget returned %#v, expected %#v", roxyOut, row.RoxyOut)
			}

			output := roxyOut.String()
			if output != row.Output {
				t.Errorf("Target.String returned %q, expected %q", output, row.Output)
			}
		})
	}
}
