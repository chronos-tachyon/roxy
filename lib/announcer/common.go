package announcer

import (
	"encoding/json"
	"errors"
	"io/fs"

	"github.com/chronos-tachyon/roxy/lib/membership"
)

func checkAnnounce(state State) {
	switch state {
	case StateReady:
		return

	case StateRunning:
		fallthrough
	case StateDead:
		panic(errors.New("Announce has already been called"))

	default:
		panic(errors.New("Close has already been called"))
	}
}

func checkWithdraw(state State) {
	switch state {
	case StateReady:
		panic(errors.New("Announce has not yet been called"))

	case StateRunning:
		fallthrough
	case StateDead:
		return

	default:
		panic(errors.New("Close has already been called"))
	}
}

func checkClose(state State) error {
	switch state {
	case StateReady:
		return nil

	case StateRunning:
		fallthrough
	case StateDead:
		panic(errors.New("Withdraw has not yet been called"))

	default:
		return fs.ErrClosed
	}
}

func convertToJSON(in *membership.Roxy, format Format, namedPort string) (raw []byte, err error) {
	var v interface{}
	switch format {
	case FinagleFormat:
		v = in.AsServerSet()
	case GRPCFormat:
		v = in.AsGRPC(namedPort)
	default:
		v = in.AsRoxyJSON()
	}

	raw, err = json.Marshal(v)
	if err != nil {
		return nil, err
	}

	return raw, nil
}
