package announcer

import (
	"encoding/json"
	"errors"
	"io/fs"

	"github.com/chronos-tachyon/roxy/lib/membership"
)

func checkAnnounce(state stateType) {
	switch state {
	case stateInit:
		return

	case stateRunning:
		fallthrough
	case stateDead:
		panic(errors.New("Announce has already been called"))

	default:
		panic(errors.New("Close has already been called"))
	}
}

func checkWithdraw(state stateType) {
	switch state {
	case stateInit:
		panic(errors.New("Announce has not yet been called"))

	case stateRunning:
		fallthrough
	case stateDead:
		return

	default:
		panic(errors.New("Close has already been called"))
	}
}

func checkClose(state stateType) error {
	switch state {
	case stateInit:
		return nil

	case stateRunning:
		fallthrough
	case stateDead:
		panic(errors.New("Withdraw has not yet been called"))

	default:
		return fs.ErrClosed
	}
}

func convertToJSON(in *membership.Roxy, format Format, namedPort string) (raw []byte, err error) {
	var v interface{}
	switch format {
	case FinagleFormat:
		v, err = in.AsServerSet()
	case GRPCFormat:
		v, err = in.AsGRPC(namedPort)
	default:
		v, err = in.AsRoxyJSON()
	}
	if err != nil {
		return nil, err
	}

	raw, err = json.Marshal(v)
	if err != nil {
		return nil, err
	}

	return raw, nil
}
