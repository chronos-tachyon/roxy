package roxyresolver

import (
	"net/url"
	"reflect"
	"testing"

	"github.com/chronos-tachyon/roxy/internal/constants"
)

func TestZKTarget_RoundTrip(t *testing.T) {
	type testRow struct {
		Input   string
		RoxyIn  Target
		ZK      ZKTarget
		RoxyOut Target
		Output  string
	}

	testData := []testRow{
		{
			Input: "zk:/path/to/dirnode",
			RoxyIn: Target{
				Scheme:     constants.SchemeZK,
				Endpoint:   "/path/to/dirnode",
				ServerName: "",
				HasSlash:   false,
			},
			ZK: ZKTarget{
				Path:       "/path/to/dirnode",
				Port:       "",
				ServerName: "",
				Balancer:   RandomBalancer,
			},
			RoxyOut: Target{
				Scheme:     constants.SchemeZK,
				Endpoint:   "/path/to/dirnode",
				Query:      url.Values{"balancer": []string{"random"}},
				ServerName: "",
				HasSlash:   true,
			},
			Output: "zk:////path/to/dirnode?balancer=random",
		},
		{
			Input: "zk:///path/to/dirnode:port?serverName=example.com",
			RoxyIn: Target{
				Scheme:     constants.SchemeZK,
				Endpoint:   "/path/to/dirnode:port",
				Query:      url.Values{"serverName": []string{"example.com"}},
				ServerName: "example.com",
				HasSlash:   true,
			},
			ZK: ZKTarget{
				Path:       "/path/to/dirnode",
				Port:       "port",
				ServerName: "example.com",
				Balancer:   RandomBalancer,
			},
			RoxyOut: Target{
				Scheme:     constants.SchemeZK,
				Endpoint:   "/path/to/dirnode:port",
				Query:      url.Values{"balancer": []string{"random"}, "serverName": []string{"example.com"}},
				ServerName: "example.com",
				HasSlash:   true,
			},
			Output: "zk:////path/to/dirnode:port?balancer=random&serverName=example.com",
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

			var target ZKTarget
			if err := target.FromTarget(roxyIn); err != nil {
				t.Errorf("ZKTarget.FromTarget failed: %v", err)
				return
			}
			if !reflect.DeepEqual(target, row.ZK) {
				t.Errorf("ZKTarget.FromTarget returned %#v, expected %#v", target, row.ZK)
			}

			roxyOut := target.AsTarget()
			if !reflect.DeepEqual(roxyOut, row.RoxyOut) {
				t.Errorf("ZKTarget.AsTarget returned %#v, expected %#v", roxyOut, row.RoxyOut)
			}

			output := roxyOut.String()
			if output != row.Output {
				t.Errorf("Target.String returned %q, expected %q", output, row.Output)
			}
		})
	}
}
