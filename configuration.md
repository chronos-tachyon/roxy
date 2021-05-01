![Roxy logo](https://chronos-tachyon.net/img/roxy-tshirt.png)

# Roxy: The Full Configuration Reference

Roxy's configuration files use [the JSON format](https://www.json.org/).
The primary config file lives at `/etc/opt/roxy/config.json`, while
the MIME rules live at `/etc/opt/roxy/mime.json`.

The overall layout of `config.json` looks like this:

```
{
  "global": {...},   # Section "global" (an Object)
  "hosts": [...],    # Section "hosts" (an Array of String)
  "targets": {...},  # Section "targets" (an Object)
  "rules": [...]     # Section "rules" (an Array of Object)
}
```

All sections are _technically_ optional, but nearly every setup will
want to define [`"hosts"`](#section-hosts),
[`"targets"`](#section-targets), and [`"rules"`](#section-rules).
Advanced users will also care about [`"global"`](#section-global).

## Full working example

Here is an example Roxy configuration for `config.json` that demonstrates both
static content serving and reverse proxying, plus a few simple header
rewrites:

```json
{
  "hosts": [
    "*.example.com",
    "subdomain.example.org",
    "other.example.org"
  ],
  "targets": {
    "fs-example-com": {
      "type": "fs",
      "path": "/srv/www/example.com/htdocs"
    },
    "fs-subdomain-example-org": {
      "type": "fs",
      "path": "/srv/www/subdomain.example.org/htdocs"
    },
    "http-other-example-org": {
      "type": "http",
      "target": "dns:///127.0.0.1:8001"
    }
  },
  "rules": [
    {
      "match": {
        "Host": "(.*\\.)?example\\.com"
      },
      "target": "fs-example-com"
    },
    {
      "match": {
        "Host": "subdomain\\.example\\.org"
      },
      "target": "fs-subdomain-example-org"
    },
    {
      "match": {
        "Host": "other\\.example\\.org"
      },
      "mutations": [
        {
          "type": "response-header-post",
          "header": "Server",
          "search": ".*",
          "replace": "Apache/2.4"
        },
        {
          "type": "response-header-post",
          "header": "Content-Security-Policy",
          "search": "(.*);",
          "replace": "\\1; image-src 'self' https://*.example.com;"
        }
      ],
      "target": "http-other-example-org"
    },
    {
      "target": "ERROR:404"
    }
  ]
}
```

***

## Section `"global"`

Section `"global"` groups together miscellaneous configuration items that don't fit in any other category.

It contains the following fields and subsections:
* [`"mimeFile"`](#field-globalmimefile)
* [`"acmeDirectoryURL"`](#field-globalacmedirectoryurl)
* [`"acmeRegistrationEmail"`](#field-globalacmeregistrationemail)
* [`"acmeUserAgent"`](#field-globalacmeuseragent)
* [`"maxCacheSize"`](#field-globalmaxcachesize)
* [`"maxComputeDigestSize"`](#field-globalmaxcomputedigestsize)
* [`"etcd"`](#subsection-globaletcd)
* [`"zk"`](#subsection-globalzk)
* [`"storage"`](#subsection-globalstorage)
* [`"pages"`](#subsection-globalpages)

```
{
  "global": {
    "mimeFile": "...",                # Path to mime.json (a String)
    "acmeDirectoryURL": "...",        # ACME server to use (a String)
    "acmeRegistrationEmail": "...",   # E-mail address to report to ACME server (a String)
    "acmeUserAgent": "...",           # User agent to report to ACME server (a String)
    "maxCacheSize": 65536,            # Max file size for in-RAM cache (a Number)
    "maxComputeDigestSize": 4194304,  # Max file size for automatic Digest/ETag headers (a Number)
    "zk": {...},                      # Subsection "global.zk" (an Object)
    "etcd": {...},                    # Subsection "global.etcd" (an Object)
    "storage": {...},                 # Subsection "global.storage" (an Object)
    "pages": {...}                    # Subsection "global.pages" (an Object)
  },
  ...
}
```

### Field `"global.mimeFile"`

Field `"global.mimeFile"` specifies the path to the `mime.json` ancillary configuration file.
If empty or not specified, it defaults to `"/etc/opt/roxy/mime.json"`.

If the file does not exist, it is not a fatal error.  Instead, Roxy will fall back on its
built-in defaults, which should be identical to the `mime.json.example` file that ships with
Roxy.

The MIME file itself is in JSON format and contains an Array of Objects.  It has the following structure:

```
[
  ...
  {
    "suffixes": ["..."],       # Literal suffixes matched against the full path (an Array of String)
    "contentType": "...",      # "Content-Type" header (a String; default `application/octet-stream`)
    "contentLanguage": "...",  # "Content-Language" header (a String; default absent)
    "contentEncoding": "..."   # "Content-Encoding" header (a String; default absent)
  },
  ...
]
```

Here's a concrete example:

```json
[
  {
    "suffixes": [".html", ".htm"],
    "contentType": "text/html; charset=utf-8",
    "contentLanguage": "en-US"
  }
]
```

You can override the "Content-Type", "Content-Language", and "Content-Encoding" headers on a
file-by-file basis using [extended attributes](https://man7.org/linux/man-pages/man7/xattr.7.html)
("xattrs"), as described in the docs for
[field `"global.maxComputeDigestSize"`](#field-globalmaxcomputedigestsize).

### Field `"global.acmeDirectoryURL"`

Field `"global.acmeDirectoryURL"` controls which ACME server endpoint Roxy uses to obtain TLS
certificates. The default is `https://acme-v02.api.letsencrypt.org/directory`, which is the
endpoint for [Let's Encrypt](https://letsencrypt.org/).

You can change it to the endpoint for another ACME provider if you wish, such as
`https://acme.zerossl.com/v2/DV90` to connect to [ZeroSSL](https://zerossl.com/).

### Field `"global.acmeRegistrationEmail"`

Field `"global.acmeRegistrationEmail"` controls the e-mail address associated with your ACME
client account.  Your ACME provider may use this e-mail from time to time to warn you about
problems with your TLS certificates.  You can omit this, if you prefer, but it is recommended.

Roxy itself does not care about your e-mail address, and will not reveal it to anyone other
than your ACME provider.

### Field `"global.acmeUserAgent"`

Field `"global.acmeUserAgent"` controls the user agent string which Roxy presents to your
ACME provider.  This helps your ACME provider track down Roxy's creators if Roxy is
misbehaving.  The default is `roxy/<version>`, which is probably what you want.

### Field `"global.maxCacheSize"`

Field `"global.maxCacheSize"` controls the maximum size of files stored in the in-memory
file cache.  The file cache is used to accelerate serving of static content, at the cost
of RAM.  The default is `65536` bytes, or 64 KiB.  You can set this to `-1` to disable
in-memory caching entirely.

NB: the file's size and last modified time are still checked on every request, even
after a cache hit.  This ensures that stale files are never served.

### Field `"global.maxComputeDigestSize"`

Field `"global.maxComputeDigestSize"` controls the maximum size of files for which Roxy
will automatically generate the "Digest" and "ETag" headers.  These headers are used for
integrity checking and for resuming interrupted downloads.  However, computing these
headers requires scanning through all bytes of the file before delivering the first byte
to the waiting HTTP client, so this is a trade-off between functionality and timeliness.
The default is `4194304` bytes, or 4 MiB, which is a trade-off chosen by the Roxy
developers to be appropriate for low-end SSDs or fast spinning disks.  You can set this
to `-1` to disable automatic computation of the "Digest" and "ETag" headers for
non-cached files.  (These headers are _always_ computed for files served from the
in-memory cache.)

NB: you can also set the "Digest" and "ETag" headers using
[ext4 extended attributes](https://man7.org/linux/man-pages/man7/xattr.7.html) ("xattrs").
Roxy respects the following xattrs:

* `user.mimetype` controls the "Content-Type" header
* `user.mimelang` controls the "Content-Language" header
* `user.mimeenc` controls the "Content-Encoding" header
* `user.md5sum` controls the "Digest: md5=" header; format is 32 lowercase hex digits
* `user.sha1sum` controls the "Digest: sha1=" header; format is 40 lowercase hex digits
* `user.sha256sum` controls the "Digest: sha-256=" header; format is 64 lowercase hex digits
* `user.etag` controls the "ETag" header directly; format is described in [RFC 7232](https://tools.ietf.org/html/rfc7232#section-2.3) but is typically a double-quoted string that is guaranteed to change whenever the file's content changes; if omitted, defaults to a function of the strongest available digest

You can set the xattrs on a file using the
[setfattr](https://man7.org/linux/man-pages/man1/setfattr.1.html) command.

### Subsection `"global.zk"`

Subsection `"global.zk"` enables the use of [Apache ZooKeeper](https://zookeeper.apache.org/) to
store TLS certificates ([subsection `"global.storage"`](#subsection-globalstorage)) and to look
up backends ([section `"targets"`](#section-targets)).  It has the following structure:

```
{
  "global": {
    ...
    "zk": {
      "servers": ["..."],       # Addresses of the ZK cluster members (an Array of String; required)
      "sessionTimeout": "30s",  # Max time that the ZK cluster should keep our session alive if we get disconnected (a String)
      "auth": {
        "scheme": "digest",     # An auth scheme; "digest" is common, other schemes are possible, see ZooKeeper docs (a String; required if "auth" present)
        "raw": "...",           # Auth data; most users will use "username" and "password" instead (a String; base-64 format)
        "username": "...",      # Username to use (a String)
        "password": "..."       # Password to use (a String)
      }
    },
    ...
  },
  ...
}
```

For a single-homed ZK cluster running on `localhost` with no authentication, this simplifies to:

```json
{
  "global": {
    "zk": {
      "servers": ["127.0.0.1:2181"]
    }
  }
}
```

Managing a ZooKeeper cluster is beyond the scope of this documentation.

### Subsection `"global.etcd"`

Subsection `"global.etcd"` enables the use of [Etcd](https://etcd.io/) to store TLS certificates
(see [subsection `"global.storage"`](#subsection-globalstorage)) and to look up backends
(see [section `"targets"`](#section-targets)).  It has the following structure:

```
{
  "global": {
    ...
    "etcd": {
      "endpoints": ["..."],     # Addresses of the etcd cluster members (an Array of String; required)
      "tls": {...},             # TLS client configuration (an Object; see below)
      "username": "...",        # Etcd username (a String)
      "password": "...",        # Etcd password (a String)
      "dialTimeout": "5s",      # Max time to wait when connecting (a String)
      "keepAliveTime": "30s",   # Time between sending of keep-alive requests (a String)
      "keepAliveTimeout": "2m"  # Max time without receiving a keep-alive reply (a String)
    },
    ...
  },
  ...
}
```

For a single-homed etcd cluster running on `localhost` with no TLS and no authentication,
this simplifies to:

```json
{
  "global": {
    "etcd": {
      "endpoints": ["http://localhost:2379"]
    }
  }
}
```

Managing an etcd cluster is beyond the scope of this documentation.

### Subsection `"global.storage"`

Subsection `"global.storage"` determines where Roxy will store TLS certificates
obtained via the ACME protocol, as well as its long-lived private key for speaking
with the ACME server.  It has the following structure:

```
{
  "global": {
    ...
    "storage": {
      "engine": "...",  # Which storage engine to use; one of "fs", "etcd", or "zk" (a String; required)
      "path": "..."     # Configuration for the storage engine (a String; required)
    },
    ...
  },
  ...
}
```

The `"global.storage.engine"` field is the name of a storage engine:

* `"fs"` uses a directory within the local filesystem; `"global.storage.path"` is a filesystem path (an absolute path is recommended)
* `"zk"` uses a directory within ZooKeeper; `"global.storage.path"` is the absolute path within the ZooKeeper cluster; `"global.zk"` is required for this engine
* `"etcd"` uses a "directory" within Etcd; `"global.storage.path"` is the absolute "path" within the Etcd cluster; `"global.etcd"` is required for this engine

(NB: Etcd 3.x does not have "paths" and "directories", per se; instead, `"path"` is suffixed with
`"/"` to form a search prefix, and each "file" is a key-value pair formed by concatenating the search
prefix with the filename.  This feels enough like a directory that the filesystem-like nomenclature
still fits, with only a few caveats.)

The default, which takes effect **only** if there is no `"global.storage"` subsection at all, is:

```json
{
  "global": {
    "storage": {
      "engine": "fs",
      "path": "/var/opt/roxy/lib/acme"
    }
  }
}
```

### Subsection `"global.pages"`

Subsection `"global.pages"` tells Roxy where to find your custom HTML templates for error pages,
redirects, and filesystem index pages.  It has the following structure:

```
{
  "global": {
    ...
    "pages": {
      "rootDir": "...",                 # Path to the template directory (a String; required)
      "defaultContentType": "...",      # Default value for the "Content-Type" header (a String)
      "defaultContentLanguage": "...",  # Default value for the "Content-Language" header (a String)
      "defaultContentEncoding": "...",  # Default value for the "Content-Encoding" header (a String)
      "map": {...}                      # Used to customize individual error codes (an Object)
    },
    ...
  },
  ...
}
```

The `"global.pages.rootDir"` field is a filesystem path that points to a directory containing
HTML templates (in [Go's `html/template` format](https://golang.org/pkg/html/template/)).

The `"global.pages.map"` field is structured as:

```
"map": {
  ...
  "<key>": {
    "fileName": "...",          # Relative path to HTML template file (a String)
    "contentType": "...",       # "Content-Type" header (a String)
    "contentLanguage": "...",   # "Content-Language" header (a String)
    "contentEncoding": "..."    # "Content-Encoding" header (a String)
  },
  ...
}
```

For HTTP 4xx and 5xx errors, the following keys are checked in `"global.pages.map"`:

* The status code, as a 3-digit numeric string
* The key `"4xx"` (for codes 400..499) or `"5xx"` (for codes 500..599)
* The key `"error"`

For HTTP 3xx redirects, the following keys are checked:

* The status code, as a 3-digit numeric string
* The key `"redir"`

For automatically-generated filesystem directory indexes, the following key is checked:

* The key `"index"`

All fields within `"global.pages.map"` are optional.  In fact, all _entries_ in the map
are optional.  If there is no entry in the map for a given key, then:

* `"fileName"` defaults to `"<key>.html"`; defaulted or not, this is interpreted relative to `"global.pages.rootDir"`
* `"contentType"` defaults to `"global.pages.defaultContentType"`
* `"contentLanguage"` defaults to `"global.pages.defaultContentLanguage"`
* `"contentEncoding"` defaults to `"global.pages.defaultContentEncoding"`

If the computed filename does not exist, then the next key is tried.  If the `"error"`,
`"redir"`, or `"index"` keys do not exist, then the Roxy built-in default templates are
used.

***

## Section `"hosts"`

Section `"hosts"` is a list of host patterns.  A host pattern is a string, which matches one of the
following patterns: a domain name, a domain name prefixed with `"*."`, or the exact string `"*"`.

This controls which domains Roxy is willing to serve, and more importantly, which domains Roxy is
willing to obtain ACME certificates for.

* A domain name, such as `"example.com"`, represents an exact match of that domain.

* A domain name prefixed with `*.`, such as `"*.example.com"`, means a match of that domain or
any subdomains beneath it.

* A literal `"*"` means that any requested domain name is considered to match.
**WARNING: This should be considered a security risk.**
Someone who doesn't like one of your websites could potentially make HTTPS requests for
domains that you don't actually own, which will cause your ACME provider to block your
webserver from obtaining future TLS certs.

## Section `"targets"`

Section `"targets"` is a map from target names (strings) to target configurations (objects).
A target name is a unique identifier that will be used by the [`"rules"` section](#section-rules)
to refer back to the target configuration.

The target configuration has the following structure:

```
{
  ...
  "targets": {
    ...
    "<name>": {
      "type": "...",    # The target type (a String; one of "fs", "http", or "grpc"; required)
      "path": "...",    # Path to the directory to serve (a String)
      "target": "...",  # Target spec for the host(s) being reverse proxied (a String)
      "tls": {...}      # TLS client configuration for connecting to the host(s) (an Object)
    },
    ...
  },
  ...
}
```

The `"<name>"` key is a unique identifier for this target configuration.  The name must
consist of letters, numbers, or the punctuation characters `_`, `.`, `+`, and `-`.  The
name cannot begin with a number or punctuation, nor can it end with punctuation, nor can
two punctuation characters appear next to each other.

The `"type"` field selects which target type to use for this target configuration:

* `"fs"` serves static content out of the local filesystem
* `"http"` provides reverse proxying to one or more hosts via HTTP (with or without TLS)
* `"grpc"` provides reverse proxying to one or more hosts via gRPC (with or without TLS)

The `"path"` field is required for `"type": "fs"`, and is forbidden for other types.  It
specifies the local filesystem directory out of which static content is served.  See
[`"global.maxCacheSize"`](#field-globalmaxcachesize) and
[`"global.maxComputeDigestSize"`](#field-globalmaxcomputedigestsize) for configuration
options.

NB: The `"fs"` type does _not_ support disabling of automatically-generated directory
indexes, and only supports index files with the exact name `index.html`.

The `"target"` field is required for `"type": "http"` and `"type": "grpc"`, and is
forbidden for other types.  It specifies the "target spec", i.e. how to connect to
the hosts being reverse proxied.  See [the "Target specs" heading](#target-specs).

The `"tls"` field is optional for `"type": "http"` and `"type": "grpc"`, and is
forbidden for other types.  The syntax is explored later in this document, under
[the "TLS client configuration" heading](#tls-client-configuration).

A simple target for static file serving might look like this:

```json
{
  "targets": {
    "my-target-name": {
      "type": "fs",
      "path": "/srv/www"
    }
  }
}
```

A target that uses mutual TLS ("mTLS") to connect to an HTTPS backend,
on the other hand, might look like this:

```json
{
  "targets": {
    "my-target-name": {
      "type": "http",
      "target": "dns:///backend.internal:443",
      "tls": {
        "clientCert": "/path/to/client/cert.pem",
        "clientKey": "/path/to/client/key.pem"
      }
    }
  }
}
```

## Section `"rules"`

Section `"rules"` is structured as an Array of Objects, where each Object represents a
single "rule".  A rule is an optional set of matching criteria, an optional list of
mutations to apply, and an optional target spec.  It does not make sense to specify a
rule with no mutations _and_ no target, but nothing prevents you from doing this.

```
"rules": [
  ...
  {
    "match": {...},      # Headers to match (Object of String)
    "mutations": [...],  # Mutations to apply to matching requests (Array of Object)
    "target": "..."      # A target name (String)
  },
  ...
]
```

### Field `"match"`

The `"match"` field consists of zero or more header matches, where each match is
composed of the name of the header (Object key) and the regular expression which
the header's value must match (Object value).  The header name is case-insensitive.

**NB: the regexp is implicitly anchored with `^` and `$`, i.e. a full string match.**

The rule only matches if **all** header matches have successfully matched against
the _original, unmodified request_, as it existed before any mutation rules.  If
any of the headers fails to match, then the entire rule is ignored.

Omitting the `"match"` field, or having `"match"` set to an empty Object, will cause
the rule to match all requests unconditionally.

There are a few special header names:

* `"Host"` matches the request hostname
* `"Method"` matches the request HTTP method
* `"Path"` matches the path part of the request HTTP URI

### Field `"mutations"`

The `"mutations"` field consists of zero or more mutations that are applied if
(and only if) the request matched the current rule.  The general structure of a
mutation object is as follows:

```
"mutations": [
  ...
  {
    "type": "...",    # The mutation type (a String; required)
    "header": "...",  # The name of the header to mutate (a String; required for some types)
    "search": "...",  # The regexp to match against the existing value (a String; required)
    "replace": "..."  # The replacement for the field's value (a String; required)
  },
  ...
]
```

The `"type"` is one of the following mutation types:

* `"request-host"` mutates the incoming request's Host
* `"request-path"` mutates the path of the incoming request's URI
* `"request-query"` mutates the query string of the incoming request's URI
* `"request-header"` mutates the named header of the incoming request
* `"response-header-pre"` mutates the named header of the outgoing response (happens early)
* `"response-header-post"` mutates the named header of the outgoing response (happens immediately before sending the reply headers)

The `"header"` field is required for the 3 mutation types that deal with headers, and is
forbidden otherwise.  It is case-insensitive.

**NB: the regexp is implicitly anchored with `^` and `$`, i.e. a full string match.**

The replacement string is a template in
[Go `"text/template"` format](https://golang.org/pkg/text/template/), which is called with
`.` equal to the `[]string` returned from calling
[`FindStringSubmatch`](https://golang.org/pkg/regexp/#Regexp.FindStringSubmatch)
on the `"search"` regexp.

**NB: As a convenience, the strings `\\0` through `\\9` are synonyms for `{{"{{"}} index . N }}` in the `"replace"` string.**

### Field `"target"`

The `"target"` field consists of the name of a target (see [Section "Targets"](#section-targets)),
or one of a handful of special strings indicating a built-in target.

If the `"target"` field is present at all, it means that **every request** which matches
the current rule will stop processing all later rules.
**No further rules will be processed (first match wins).**  Conversely, if no `"target"`
field is present, then processing continues to the next matching rule.

Here are the special built-in targets:

* `"ERROR:<status>"` causes Roxy to fail the request with the given 4xx or 5xx status code
* `"REDIR:<status>:<url>"` causes Roxy to send a redirect with the given 3xx status code and URL

The `<url>` in `REDIR:<status>:<url>` is a template in
[Go `"text/template"` format](https://golang.org/pkg/text/template/), which is called with
`.` equal to a [`*url.URL`](https://golang.org/pkg/net/url/#URL) representing the current
request URI, with scheme and host filled in, and with all mutations made up to this point.
This means that, if you want to rewrite the URL and then redirect the client to the mutated
URL, `"target": "STATUS:302:{{.}}"` will do nicely.

***

## Target specs

A target spec is defined similarly to
[the gRPC Name Syntax](https://github.com/grpc/grpc/blob/master/doc/naming.md):
a target spec is a URL-like string which contains an optional "scheme", an
optional "authority", a mandatory "target", and an optional "query".

* `"some_string"` is interpreted as `(nil, nil, "some_string", nil)`
* `"some:string"` is interpreted as `("some", nil, "string", nil)`
* `"some:/string"` is interpreted as `("some", nil, "/string", nil)`
* `"some:///string"` is interpreted as `("some", nil, "string", nil)`
* `"some:////string"` is interpreted as `("some", nil, "/string", nil)`
* `"some://long/string"` is interpreted as `("some", "long", "string", nil)`
* `"some://long/string?q=1"` is interpreted as `("some", "long", "string", "q=1")`

The meaning of the authority, target, and query depends on the scheme.

The following schemes are supported, both for HTTP and for gRPC:

### Scheme `"dns"`

Complete syntax: `"dns://<server:port>/<domain:port>?balancer=<algo>&pollInterval=<dur>&serverName=<name>"`

The `<server:port>` (of the optional authority string) names a specific DNS
server, overriding your OS `/etc/resolv.conf` settings.  If this is present
at all, the port is almost always 53, which is the default.  Very few people
will want to specify this.

The `<domain:port>` (of the mandatory target string) is the domain name to
resolve, plus a named or numbered port.  The `domain` specifies the A/AAAA
records to query.  The `port` is optional, with a default of `80` without
TLS or `443` with TLS.  All hosts must use the same port.

The optional `balancer=<algo>` query parameter specifies the balancer
algorithm to use. See [the "Balancer algorithms" heading](#balancer-algorithms).

The optional `pollInterval=<dur>` query parameter specifies how long to
cache the DNS query results in memory before making another query.  The
default is a fairly aggressive `"1m"` (one minute).

The optional `serverName=<name>` query parameter specifies the expected
DNSName SAN on the TLS certificate, overriding the default of `domain`.
This only has an effect when TLS is in use.

### Scheme `"srv"`

Complete syntax: `srv://<server:port>/<domain>/<service>?balancer=<algo>&pollInterval=<dur>&serverName=<name>`

The `<server:port>` (of the optional authority string) names a specific DNS
server, overriding your OS `/etc/resolv.conf` settings.  If this is present
at all, the port is almost always 53, which is the default.  Very few people
will want to specify this.

The `<domain>/<service>` (of the mandatory target string) is used to
construct the domain name to resolve.  Both parts are mandatory.  DNS SRV
records will be queried at `_<service>._tcp.<domain>`, followed by lookups
of the A/AAAA records of the resulting domain names.  Each SRV record
specifies its own port, as well as a priority and weight that can be used
by a special balancer algorithm.

The optional `balancer=<algo>` query parameter specifies the balancer
algorithm to use. See [the "Balancer algorithms" heading](#balancer-algorithms).

The optional `pollInterval=<dur>` query parameter specifies how long to
cache the DNS query results in memory before making another query.  The
default is a fairly aggressive `"1m"` (one minute).

The optional `serverName=<name>` query parameter specifies the expected
DNSName SAN on the TLS certificate, overriding the default of the domain(s)
named by the SRV records.  (That is, the default ServerName is the name of
the A/AAAA records, not the name of the SRV records.)  This only has an
effect when TLS is in use.

### Scheme `"zk"`

Complete syntax: `zk:///path/to/dir:namedPort?balancer=<algo>`

The authority section must be empty.

The mandatory `path/to/dir` (of the mandatory target string) specifies the path to
a ZooKeeper directory containing files, one per host, in either of two
JSON syntaxes:
* [Finagle ServerSet](https://github.com/twitter/finagle)
* [Etcd gRPC Naming](https://etcd.io/docs/v3.3/dev-guide/grpc_naming/)

If `path/to/dir` does _not_ begin with a `/`, one is automatically added for you.

The optional `portName` (of the mandatory target string) names a port within the Finagle ServerSet data.

The optional `balancer=<algo>` query parameter specifies the balancer
algorithm to use. See [the "Balancer algorithms" heading](#balancer-algorithms).

### Scheme `"etcd"`

Complete syntax: `etcd:///prefix/string:namedPort?balancer=<algo>`

The mandatory `prefix/string` (of the mandatory target string) specifies the prefix
of an Etcd keyspace containing key-value pairs, one per host, in either of two
JSON syntaxes:
* [Finagle ServerSet](https://github.com/twitter/finagle)
* [Etcd gRPC Naming](https://etcd.io/docs/v3.3/dev-guide/grpc_naming/)

If `prefix/string` does _not_ begin with a `/`, it **will not** be added for you.
Etcd 3.x allows keys whose names don't start with `/`, even though the `/` is
conventional.  If your `prefix/string` starts with a `/`, use four slashes between
the scheme and the target, like so: `etcd:////path/to/dir`.

The `prefix/string` _must not_ end with a `/`.  One will be automatically added
for you.

The optional `balancer=<algo>` query parameter specifies the balancer
algorithm to use. See [the "Balancer algorithms" heading](#balancer-algorithms).

***

## Balancer algorithms

### Balancer `"random"`

This balancer picks a host at random with uniform probability.

### Balancer `"roundRobin"`

This balancer shuffles the hosts into a random permutation, then cycles through
the hosts in that order until the next resolver change.  Each host has uniform
probability.

### Balancer `"leastLoaded"`

This balancer is not fully implemented yet.  It currently behaves similarly to
the `"random"` balancer.

### Balancer `"srv"`

This balancer is only valid for [the `"srv"` target scheme](#scheme-srv).  It
respects the SRV records' priority and weight fields to do weighted, non-uniform
random selection.  As health checking has not yet been implemented, it will
never use any priority tier except the one with lowest numeric value.

***

## TLS client configuration

Some sections, such as [`"global.etcd"`](#subsection-globaletcd) and
[`"targets"`](#section-targets), optionally take a `"tls"` block to specify (1) that TLS
should be used, and (2) how to configure it.  It has the following structure:

```
...
"tls": {
  "skipVerify": bool,         # Bool, if true then no validation whatsoever is performed on the server's certificate
  "skipVerifyDNSName": bool,  # Bool, if true then no checking is done that the TLS server's certificate matches ServerName (rootCA and exactCN are still checked)
  "rootCA": "...",            # String, if not empty then it contains the path to the trusted root CAs as a concatenated PEM file; default is to use the system trusted roots
  "exactCN": "...",           # String, if not empty then it contains the CommonName which must match the TLS server's certificate subject name
  "forceDNSName": "...",      # String, if not empty then it contains the DNSName or IPAddress which must match the TLS server's certificate extensions
  "clientCert": "...",        # String, if not empty then it contains the path to a PEM file containing your client cert
  "clientKey": "..."          # String, if not empty then it contains the path to a PEM file containing your client private key; default is to check clientCert
},
...
```

All fields are optional and have reasonable defaults.  The simplest configuration, in
which TLS is used with all and only the standard verification steps, and with no
mutual TLS ("mTLS"), is as follows:

```
...
"tls": {},
...
```
