package roxyresolver

import (
	"net"
	"net/url"
	"reflect"
	"testing"

	"github.com/chronos-tachyon/roxy/internal/constants"
)

func TestIPTarget_RoundTrip(t *testing.T) {
	type testRow struct {
		Input   string
		RoxyIn  Target
		IP      IPTarget
		RoxyOut Target
		Output  string
	}

	testData := []testRow{
		{
			Input: "ip:127.0.0.1",
			RoxyIn: Target{
				Scheme:     constants.SchemeIP,
				Endpoint:   "127.0.0.1",
				ServerName: "127.0.0.1",
				HasSlash:   false,
			},
			IP: IPTarget{
				Addrs: []*net.TCPAddr{
					{IP: net.IP{127, 0, 0, 1}, Port: 443},
				},
				ServerName: "127.0.0.1",
				Balancer:   RandomBalancer,
			},
			RoxyOut: Target{
				Scheme:     constants.SchemeIP,
				Endpoint:   "127.0.0.1:443",
				Query:      url.Values{"balancer": []string{"random"}, "serverName": []string{"127.0.0.1"}},
				ServerName: "127.0.0.1",
				HasSlash:   true,
			},
			Output: "ip:///127.0.0.1:443?balancer=random&serverName=127.0.0.1",
		},
		{
			Input: "ip:8.8.8.8:53?balancer=rr",
			RoxyIn: Target{
				Scheme:     constants.SchemeIP,
				Endpoint:   "8.8.8.8:53",
				Query:      url.Values{"balancer": []string{"rr"}},
				ServerName: "8.8.8.8",
				HasSlash:   false,
			},
			IP: IPTarget{
				Addrs: []*net.TCPAddr{
					{IP: net.IP{8, 8, 8, 8}, Port: 53},
				},
				ServerName: "8.8.8.8",
				Balancer:   RoundRobinBalancer,
			},
			RoxyOut: Target{
				Scheme:     constants.SchemeIP,
				Endpoint:   "8.8.8.8:53",
				Query:      url.Values{"balancer": []string{"roundRobin"}, "serverName": []string{"8.8.8.8"}},
				ServerName: "8.8.8.8",
				HasSlash:   true,
			},
			Output: "ip:///8.8.8.8:53?balancer=roundRobin&serverName=8.8.8.8",
		},
		{
			Input: "ip:8.8.8.8:53,8.8.4.4:53",
			RoxyIn: Target{
				Scheme:     constants.SchemeIP,
				Endpoint:   "8.8.8.8:53,8.8.4.4:53",
				ServerName: "",
				HasSlash:   false,
			},
			IP: IPTarget{
				Addrs: []*net.TCPAddr{
					{IP: net.IP{8, 8, 8, 8}, Port: 53},
					{IP: net.IP{8, 8, 4, 4}, Port: 53},
				},
				ServerName: "",
				Balancer:   RandomBalancer,
			},
			RoxyOut: Target{
				Scheme:     constants.SchemeIP,
				Endpoint:   "8.8.8.8:53,8.8.4.4:53",
				Query:      url.Values{"balancer": []string{"random"}},
				ServerName: "",
				HasSlash:   true,
			},
			Output: "ip:///8.8.8.8:53,8.8.4.4:53?balancer=random",
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

			var target IPTarget
			if err := target.FromTarget(roxyIn, constants.PortHTTPS); err != nil {
				t.Errorf("IPTarget.FromTarget failed: %v", err)
				return
			}
			if !reflect.DeepEqual(target, row.IP) {
				t.Errorf("IPTarget.FromTarget returned %#v, expected %#v", target, row.IP)
			}

			roxyOut := target.AsTarget()
			if !reflect.DeepEqual(roxyOut, row.RoxyOut) {
				t.Errorf("IPTarget.AsTarget returned %#v, expected %#v", roxyOut, row.RoxyOut)
			}

			output := roxyOut.String()
			if output != row.Output {
				t.Errorf("Target.String returned %q, expected %q", output, row.Output)
			}
		})
	}
}
