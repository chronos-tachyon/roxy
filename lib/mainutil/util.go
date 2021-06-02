package mainutil

import (
	"strings"
)

const (
	optionAllow            = "allow"
	optionAuthData         = "authdata"
	optionAuthType         = "authtype"
	optionCA               = "ca"
	optionClientCA         = "clientCA"
	optionServerCA         = "serverCA"
	optionCert             = "cert"
	optionClientCert       = "clientCert"
	optionServerCert       = "serverCert"
	optionCN               = "cn"
	optionCommonName       = "commonName"
	optionDialTimeout      = "dialTimeout"
	optionFormat           = "format"
	optionKeepAlive        = "keepAlive"
	optionKeepAliveTimeout = "keepAliveTimeout"
	optionKey              = "key"
	optionClientKey        = "clientKey"
	optionServerKey        = "serverKey"
	optionLoc              = "loc"
	optionLocation         = "location"
	optionMTLS             = "mtls"
	optionName             = "name"
	optionNet              = "net"
	optionNetwork          = "network"
	optionPassword         = "password"
	optionPath             = "path"
	optionPort             = "port"
	optionSN               = "sn"
	optionServerName       = "serverName"
	optionSessionTimeout   = "sessionTimeout"
	optionTLS              = "tls"
	optionHostID           = "hostID"
	optionUniqueID         = "uniqueID"
	optionUsername         = "username"
	optionVerify           = "verify"
	optionVerifyServerName = "verifyServerName"

	optionValueOn = "on"
)

var incompleteOptions = map[string]string{
	optionTLS:              optionValueOn,
	optionMTLS:             optionValueOn,
	optionVerify:           optionValueOn,
	optionVerifyServerName: optionValueOn,
}

func splitOption(str string) (string, string, bool, error) {
	i := strings.IndexByte(str, '=')
	if i >= 0 {
		return str[:i], str[i+1:], true, nil
	}

	name := str
	value, found := incompleteOptions[name]
	if found {
		return name, value, false, nil
	}

	return name, "", false, OptionError{
		Name:     name,
		Value:    "",
		Complete: false,
		Err:      MissingOptionValueError{},
	}
}
