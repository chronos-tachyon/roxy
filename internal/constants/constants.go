package constants

// Various constants.
const (
	// NullString is the string representation of the JSON null value.
	NullString = "null"

	// NetEmpty et al are networks for net.Dial() and friends.
	NetEmpty      = ""
	NetUDP        = "udp"
	NetUDP4       = "udp4"
	NetUDP6       = "udp6"
	NetTCP        = "tcp"
	NetTCP4       = "tcp4"
	NetTCP6       = "tcp6"
	NetUnix       = "unix"
	NetUnixgram   = "unixgram"
	NetUnixPacket = "unixpacket"

	// SchemeHTTP et al are URL schemes.
	SchemeHTTP  = "http"
	SchemeHTTPS = "https"

	// SchemeEmpty et al are RoxyTarget schemes.
	SchemeEmpty        = ""
	SchemePassthrough  = "passthrough"
	SchemeUnix         = "unix"
	SchemeUnixAbstract = "unix-abstract"
	SchemeIP           = "ip"
	SchemeDNS          = "dns"
	SchemeSRV          = "srv"
	SchemeZK           = "zk"
	SchemeEtcd         = "etcd"
	SchemeATC          = "atc"

	// PortDNS et al are TCP or UDP port numbers, in decimal string form.
	PortDNS   = "53"
	PortHTTP  = "80"
	PortHTTPS = "443"
	PortZK    = "2181"
	PortEtcd  = "2379"
	PortATC   = "2987"

	// SubsystemAdmin et al are server subsystem names.
	SubsystemAdmin = "admin"
	SubsystemProm  = "prom"
	SubsystemHTTP  = "http"
	SubsystemHTTPS = "https"
	SubsystemGRPC  = "grpc"
)

var (
	// NullBytes is the []byte representation of the JSON null value.
	NullBytes = []byte("null")
)

// IsNetUnix returns true iff its argument is an AF_UNIX network.
func IsNetUnix(str string) bool {
	switch str {
	case NetUnix:
		return true
	case NetUnixgram:
		return true
	case NetUnixPacket:
		return true
	default:
		return false
	}
}

// IsNetUDP returns true iff its argument is an AF_INET/AF_INET6 network with
// IPPROTO_UDP.
func IsNetUDP(str string) bool {
	switch str {
	case NetUDP:
		return true
	case NetUDP4:
		return true
	case NetUDP6:
		return true
	default:
		return false
	}
}

// IsNetTCP returns true iff its argument is an AF_INET/AF_INET6 network with
// IPPROTO_TCP.
func IsNetTCP(str string) bool {
	switch str {
	case NetTCP:
		return true
	case NetTCP4:
		return true
	case NetTCP6:
		return true
	default:
		return false
	}
}
