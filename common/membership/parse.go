package membership

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	multierror "github.com/hashicorp/go-multierror"
)

func (ss *ServerSet) Parse(raw []byte) error {
	if ss == nil {
		panic(errors.New("ss is nil"))
	}

	if string(raw) == "null" {
		return nil
	}

	*ss = ServerSet{ShardID: -1}

	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	d.DisallowUnknownFields()
	err0 := d.Decode(ss)
	if err0 == nil {
		if err := fixIPAndZone(ss); err != nil {
			*ss = ServerSet{}
			return err
		}
		return nil
	}

	*ss = ServerSet{ShardID: -1}

	var grpc GRPC
	d = json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	d.DisallowUnknownFields()
	err1 := d.Decode(&grpc)
	if err1 == nil {
		ss.Status = StatusDead

		if grpc.Op == GRPCOpAdd {
			host, port, err := net.SplitHostPort(grpc.Addr)
			if err != nil {
				return fmt.Errorf("failed to SplitHostPort: %q: %w", grpc.Addr, err)
			}

			portNum, err := strconv.ParseUint(port, 10, 16)
			if err != nil {
				return fmt.Errorf("failed to ParseUint on port: %q: %w", port, err)
			}

			ss.Status = StatusAlive

			ss.ServiceEndpoint = &ServerSetEndpoint{
				Host: host,
				Port: uint16(portNum),
			}

			if x, ok := grpc.Metadata.(map[string]interface{}); ok {
				ss.Metadata = make(map[string]string, len(x))

				for k, v := range x {
					if str, ok := v.(string); ok {
						ss.Metadata[k] = str
					}
				}

				if y, ok := x["ShardID"].(json.Number); ok {
					shardID, err := strconv.ParseInt(string(y), 10, 32)
					if err == nil {
						ss.ShardID = int32(shardID)
					}
				}
			}

			if err := fixIPAndZone(ss); err != nil {
				*ss = ServerSet{}
				return err
			}
		}

		return nil
	}

	return fmt.Errorf("failed to parse JSON: `%s`: %w", string(raw), err0)
}

func fixIPAndZone(out *ServerSet) error {
	var errs multierror.Error
	errs.Errors = make([]error, 0, 1+len(out.AdditionalEndpoints))
	if err := fixIPAndZoneForEndpoint(out.ServiceEndpoint); err != nil {
		errs.Errors = append(errs.Errors, err)
	}
	for _, ep := range out.AdditionalEndpoints {
		if err := fixIPAndZoneForEndpoint(ep); err != nil {
			errs.Errors = append(errs.Errors, err)
		}
	}
	return errs.ErrorOrNil()
}

func fixIPAndZoneForEndpoint(ep *ServerSetEndpoint) error {
	if ep == nil {
		return nil
	}

	var (
		ipStr string
		zone  string
	)
	if i := strings.IndexByte(ep.Host, '%'); i >= 0 {
		ipStr, zone = ep.Host[:i], ep.Host[i+1:]
	} else {
		ipStr = ep.Host
	}

	ep.IP = net.ParseIP(ipStr)
	ep.Zone = zone

	if ep.IP == nil {
		return fmt.Errorf("failed to ParseIP: %q", ipStr)
	}
	if ep.Port == 0 {
		return fmt.Errorf("invalid port number 0")
	}
	return nil
}
