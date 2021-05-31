package roxyresolver

import (
	"net/url"
	"reflect"
	"testing"

	"github.com/chronos-tachyon/roxy/internal/constants"
)

func TestEtcdTarget_RoundTrip(t *testing.T) {
	type testRow struct {
		Input   string
		RoxyIn  Target
		Etcd    EtcdTarget
		RoxyOut Target
		Output  string
	}

	testData := []testRow{
		{
			Input: "etcd:/path/prefix/",
			RoxyIn: Target{
				Scheme:     constants.SchemeEtcd,
				Endpoint:   "/path/prefix/",
				ServerName: "",
				HasSlash:   false,
			},
			Etcd: EtcdTarget{
				Path:       "/path/prefix/",
				Port:       "",
				ServerName: "",
				Balancer:   RandomBalancer,
			},
			RoxyOut: Target{
				Scheme:     constants.SchemeEtcd,
				Endpoint:   "/path/prefix/",
				Query:      url.Values{"balancer": []string{"random"}},
				ServerName: "",
				HasSlash:   true,
			},
			Output: "etcd:////path/prefix/?balancer=random",
		},
		{
			Input: "etcd:///path/prefix:port?serverName=example.com",
			RoxyIn: Target{
				Scheme:     constants.SchemeEtcd,
				Endpoint:   "path/prefix:port",
				Query:      url.Values{"serverName": []string{"example.com"}},
				ServerName: "example.com",
				HasSlash:   true,
			},
			Etcd: EtcdTarget{
				Path:       "path/prefix/",
				Port:       "port",
				ServerName: "example.com",
				Balancer:   RandomBalancer,
			},
			RoxyOut: Target{
				Scheme:     constants.SchemeEtcd,
				Endpoint:   "path/prefix/:port",
				Query:      url.Values{"balancer": []string{"random"}, "serverName": []string{"example.com"}},
				ServerName: "example.com",
				HasSlash:   true,
			},
			Output: "etcd:///path/prefix/:port?balancer=random&serverName=example.com",
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

			var target EtcdTarget
			if err := target.FromTarget(roxyIn); err != nil {
				t.Errorf("EtcdTarget.FromTarget failed: %v", err)
				return
			}
			if !reflect.DeepEqual(target, row.Etcd) {
				t.Errorf("EtcdTarget.FromTarget returned %#v, expected %#v", target, row.Etcd)
			}

			roxyOut := target.AsTarget()
			if !reflect.DeepEqual(roxyOut, row.RoxyOut) {
				t.Errorf("EtcdTarget.AsTarget returned %#v, expected %#v", roxyOut, row.RoxyOut)
			}

			output := roxyOut.String()
			if output != row.Output {
				t.Errorf("Target.String returned %q, expected %q", output, row.Output)
			}
		})
	}
}
