package mainutil

import (
	"io"
	"net"
	"os"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/chronos-tachyon/roxy/internal/constants"
)

func sdNotify(payload string) {
	// https://www.freedesktop.org/software/systemd/man/sd_notify.html

	name, ok := os.LookupEnv("NOTIFY_SOCKET")
	if !ok {
		return
	}
	if len(name) == 0 {
		return
	}

	realName := name
	if name[0] == '@' {
		realName = "\x00" + name[1:]
	}

	unixAddr := &net.UnixAddr{
		Net:  constants.NetUnixgram,
		Name: realName,
	}

	conn, err := net.DialUnix(unixAddr.Net, nil, unixAddr)
	if err != nil {
		log.Logger.Warn().
			Str("socket", name).
			Err(err).
			Msg("sdNotify: failed to Dial")
		return
	}
	defer func() {
		_ = conn.Close()
	}()

	_, err = io.Copy(conn, strings.NewReader(payload))
	if err != nil {
		log.Logger.Warn().
			Str("socket", name).
			Err(err).
			Msg("sdNotify: failed to Write")
		return
	}

	log.Logger.Trace().
		Str("socket", name).
		Str("payload", payload).
		Msg("sdNotify: success")
}
