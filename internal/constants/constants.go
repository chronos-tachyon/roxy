package constants

// NullString is the string representation of the JSON null value.
const NullString = "null"

// NullBytes is the []byte representation of the JSON null value.
var NullBytes = []byte("null")

// NetEmpty et al are networks for net.Dial() and friends.
const (
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
)

// SchemeHTTP et al are URL schemes.
const (
	SchemeHTTP  = "http"
	SchemeHTTPS = "https"
)

// SchemeEmpty et al are RoxyTarget schemes.
const (
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
)

// PortDNS et al are TCP or UDP port numbers, in decimal string form.
const (
	PortDNS   = "53"
	PortHTTP  = "80"
	PortHTTPS = "443"
	PortZK    = "2181"
	PortEtcd  = "2379"
	PortATC   = "2987"
)

// SubsystemAdmin et al are server subsystem names.
const (
	SubsystemAdmin = "admin"
	SubsystemProm  = "prom"
	SubsystemHTTP  = "http"
	SubsystemHTTPS = "https"
	SubsystemGRPC  = "grpc"
)

// HTTP methods.
const (
	MethodOTHER = "OTHER"
)

// HTTP status codes (as strings).
const (
	Status1XX = "1XX"
	Status100 = "100"
	Status101 = "101"
	Status2XX = "2XX"
	Status200 = "200"
	Status204 = "204"
	Status206 = "206"
	Status3XX = "3XX"
	Status301 = "301"
	Status302 = "302"
	Status304 = "304"
	Status307 = "307"
	Status308 = "308"
	Status4XX = "4XX"
	Status400 = "400"
	Status401 = "401"
	Status403 = "403"
	Status404 = "404"
	Status405 = "405"
	Status5XX = "5XX"
	Status500 = "500"
	Status503 = "503"
)

// HTTP headers.
const (
	HeaderAllow        = "allow"
	HeaderCacheControl = "cache-control"
	HeaderContentEnc   = "content-encoding"
	HeaderContentLang  = "content-language"
	HeaderContentLen   = "content-length"
	HeaderCSP          = "content-security-policy"
	HeaderContentType  = "content-type"
	HeaderDigest       = "digest"
	HeaderETag         = "etag"
	HeaderForwarded    = "forwarded"
	HeaderLastModified = "last-modified"
	HeaderLocation     = "location"
	HeaderOrigin       = "origin"
	HeaderReferer      = "referer"
	HeaderRoxyFrontend = "roxy-frontend"
	HeaderServer       = "server"
	HeaderSourceMap    = "sourcemap"
	HeaderHSTS         = "strict-transport-security"
	HeaderUserAgent    = "user-agent"
	HeaderXCTO         = "x-content-type-options"
	HeaderXFwdFor      = "x-forwarded-for"
	HeaderXFwdHost     = "x-forwarded-host"
	HeaderXFwdIP       = "x-forwarded-ip"
	HeaderXFwdProto    = "x-forwarded-proto"
	HeaderXXSSP        = "x-xss-protection"
	HeaderXID          = "xid"
)

// HTTP/2 pseudo-headers.
const (
	PseudoHeaderScheme    = ":scheme"
	PseudoHeaderMethod    = ":method"
	PseudoHeaderAuthority = ":authority"
	PseudoHeaderPath      = ":path"
	PseudoHeaderStatus    = ":status"
)

// Cache-Control header values.
const (
	CacheControlNoCache    = "no-cache"
	CacheControl1Day       = "max-age=86400, must-revalidate"
	CacheControlPublic1Day = "public, max-age=86400, must-revalidate"
)

// Content-Type header values.
const (
	ContentTypeTextPlain      = "text/plain; charset=utf-8"
	ContentTypeTextHTML       = "text/html; charset=utf-8"
	ContentTypeAppOctetStream = "application/octet-stream"
	ContentTypeAppJS          = "application/javascript"
	ContentTypeTextJS         = "text/javascript"
)

// Content-Language header values.
const (
	ContentLangEN   = "en"
	ContentLangENUS = "en-US"
)

// Allow header values.
const (
	AllowGET     = "OPTIONS, GET, HEAD"
	AllowGETPOST = "OPTIONS, GET, HEAD, POST"
	AllowGPD     = "OPTIONS, GET, HEAD, PUT, DELETE"
	AllowGPPD    = "OPTIONS, GET, HEAD, POST, PUT, DELETE"
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
