package roxyresolver

import (
	"net"
	"net/url"
	"reflect"
	"testing"

	"github.com/chronos-tachyon/roxy/internal/constants"
)

func TestSRVTarget_RoundTrip(t *testing.T) {
	type testRow struct {
		Input   string
		RoxyIn  Target
		SRV     SRVTarget
		RoxyOut Target
		Output  string
	}

	testData := []testRow{
		{
			Input: "srv:example.com/http",
			RoxyIn: Target{
				Scheme:     constants.SchemeSRV,
				Endpoint:   "example.com/http",
				ServerName: "",
				HasSlash:   false,
			},
			SRV: SRVTarget{
				Domain:     "example.com",
				Service:    "http",
				ServerName: "",
				Balancer:   RandomBalancer,
			},
			RoxyOut: Target{
				Scheme:     constants.SchemeSRV,
				Endpoint:   "example.com/http",
				Query:      url.Values{"balancer": []string{"random"}},
				ServerName: "",
				HasSlash:   true,
			},
			Output: "srv:///example.com/http?balancer=random",
		},
		{
			Input: "srv://8.8.8.8/example.com/http",
			RoxyIn: Target{
				Scheme:     constants.SchemeSRV,
				Authority:  "8.8.8.8",
				Endpoint:   "example.com/http",
				ServerName: "",
				HasSlash:   true,
			},
			SRV: SRVTarget{
				ResolverAddr: &net.TCPAddr{IP: net.IP{8, 8, 8, 8}, Port: 53},
				Domain:       "example.com",
				Service:      "http",
				ServerName:   "",
				Balancer:     RandomBalancer,
			},
			RoxyOut: Target{
				Scheme:     constants.SchemeSRV,
				Authority:  "8.8.8.8:53",
				Endpoint:   "example.com/http",
				Query:      url.Values{"balancer": []string{"random"}},
				ServerName: "",
				HasSlash:   true,
			},
			Output: "srv://8.8.8.8:53/example.com/http?balancer=random",
		},
		{
			Input: "srv:///_service._tcp.example.com?serverName=example.com",
			RoxyIn: Target{
				Scheme:     constants.SchemeSRV,
				Endpoint:   "_service._tcp.example.com",
				Query:      url.Values{"serverName": []string{"example.com"}},
				ServerName: "example.com",
				HasSlash:   true,
			},
			SRV: SRVTarget{
				Domain:     "_service._tcp.example.com",
				Service:    "",
				ServerName: "example.com",
				Balancer:   RandomBalancer,
			},
			RoxyOut: Target{
				Scheme:     constants.SchemeSRV,
				Endpoint:   "_service._tcp.example.com",
				Query:      url.Values{"balancer": []string{"random"}, "serverName": []string{"example.com"}},
				ServerName: "example.com",
				HasSlash:   true,
			},
			Output: "srv:///_service._tcp.example.com?balancer=random&serverName=example.com",
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

			var target SRVTarget
			if err := target.FromTarget(roxyIn); err != nil {
				t.Errorf("SRVTarget.FromTarget failed: %v", err)
				return
			}
			if !reflect.DeepEqual(target, row.SRV) {
				t.Errorf("SRVTarget.FromTarget returned %#v, expected %#v", target, row.SRV)
			}

			roxyOut := target.AsTarget()
			if !reflect.DeepEqual(roxyOut, row.RoxyOut) {
				t.Errorf("SRVTarget.AsTarget returned %#v, expected %#v", roxyOut, row.RoxyOut)
			}

			output := roxyOut.String()
			if output != row.Output {
				t.Errorf("Target.String returned %q, expected %q", output, row.Output)
			}
		})
	}
}
