package roxyresolver

import (
	"net/url"
	"reflect"
	"testing"

	"github.com/chronos-tachyon/roxy/internal/constants"
)

func TestATCTarget_RoundTrip(t *testing.T) {
	type testRow struct {
		Input   string
		RoxyIn  Target
		ATC     ATCTarget
		RoxyOut Target
		Output  string
	}

	testData := []testRow{
		{
			Input: "atc:service?unique=foo",
			RoxyIn: Target{
				Scheme:     constants.SchemeATC,
				Endpoint:   "service",
				Query:      url.Values{"unique": []string{"foo"}},
				ServerName: "",
				HasSlash:   false,
			},
			ATC: ATCTarget{
				ServiceName:    "service",
				ShardNumber:    0,
				HasShardNumber: false,
				UniqueID:       "foo",
				Location:       "",
				ServerName:     "",
				Balancer:       WeightedRoundRobinBalancer,
				CPS:            0.0,
			},
			RoxyOut: Target{
				Scheme:     constants.SchemeATC,
				Endpoint:   "service",
				Query:      url.Values{"unique": []string{"foo"}},
				ServerName: "",
				HasSlash:   true,
			},
			Output: "atc:///service?unique=foo",
		},
		{
			Input: "atc:///service/42?balancer=rand&unique=foo&location=aws:us-east-1&cps=50",
			RoxyIn: Target{
				Scheme:     constants.SchemeATC,
				Endpoint:   "service/42",
				Query:      url.Values{"balancer": []string{"rand"}, "unique": []string{"foo"}, "location": []string{"aws:us-east-1"}, "cps": []string{"50"}},
				ServerName: "",
				HasSlash:   true,
			},
			ATC: ATCTarget{
				ServiceName:    "service",
				ShardNumber:    42,
				HasShardNumber: true,
				UniqueID:       "foo",
				Location:       "aws:us-east-1",
				ServerName:     "",
				Balancer:       RandomBalancer,
				CPS:            50.0,
			},
			RoxyOut: Target{
				Scheme:     constants.SchemeATC,
				Endpoint:   "service/42",
				Query:      url.Values{"balancer": []string{"random"}, "unique": []string{"foo"}, "location": []string{"aws:us-east-1"}, "cps": []string{"50"}},
				ServerName: "",
				HasSlash:   true,
			},
			Output: "atc:///service/42?balancer=random&cps=50&location=aws%3Aus-east-1&unique=foo",
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

			var target ATCTarget
			if err := target.FromTarget(roxyIn); err != nil {
				t.Errorf("ATCTarget.FromTarget failed: %v", err)
				return
			}
			if !reflect.DeepEqual(target, row.ATC) {
				t.Errorf("ATCTarget.FromTarget returned %#v, expected %#v", target, row.ATC)
			}

			roxyOut := target.AsTarget()
			if !reflect.DeepEqual(roxyOut, row.RoxyOut) {
				t.Errorf("ATCTarget.AsTarget returned %#v, expected %#v", roxyOut, row.RoxyOut)
			}

			output := roxyOut.String()
			if output != row.Output {
				t.Errorf("Target.String returned %q, expected %q", output, row.Output)
			}
		})
	}
}
