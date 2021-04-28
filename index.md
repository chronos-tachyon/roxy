## Roxy the Frontend Proxy

> ![Roxy Lalonde](https://chronos-tachyon.net/img/roxy-lalonde.png)
> 
> Our mascot, [Roxy Lalonde](https://mspaintadventures.fandom.com/wiki/Roxy_Lalonde).

Roxy is an Internet-facing HTTPS frontend proxy that's meant to scale from
hobbyist websites to very large installations, assuming that you're a fan of
[Let's Encrypt](https://letsencrypt.org/) for your TLS certificates.

Roxy supports basic serving of files directly from the filesystem, so that you
don't need to also install Apache, lighttpd, nginx, or some other HTTP server
just to serve your static content.  It does not support, and will never
support, scripting engines in the style of PHP, `mod_python`, etc.  Instead,
you are invited to run your web application as its own webserver _behind_
Roxy, and Roxy will take care of all the Internet-facing stuff that you would
normally have to re-implement yourself, such as TLS certificates and modern
security-hardening HTTP headers.

Roxy currently supports HTTP, HTTPS, and gRPC (over both TLS and plaintext) to
communicate with backend web servers.

## Installing with Docker

```sh
docker pull chronostachyon/roxy
# Set up configuration in /etc/opt/roxy on the host
# Prepare /var/opt/roxy/lib/acme on the host
# Static content, if any, goes in /srv/www
docker run --rm -it --name roxy \
  -v /var/opt/roxy/lib/acme:/var/opt/roxy/lib/acme \
  -v /etc/opt/roxy:/etc/opt/roxy:ro \
  -v /srv/www:/srv/www:ro \
  -p 80 -p 443 \
  chronostachyon/roxy
```

## Installing with APT (Debian/Ubuntu)

```sh
sudo curl -fsSLR -o /etc/apt/trusted.gpg.d/roxy.gpg https://apt.chronos-tachyon.net/keys.gpg
# Or "curl https://apt.chronos-tachyon.net/keys.gpg | sudo apt-key add -"
echo 'deb https://apt.chronos-tachyon.net roxy main' | sudo tee /etc/apt/sources.list.d/roxy.list
sudo apt update
sudo apt install roxy
```

***

## Configuring Roxy

Roxy is configured using JSON format.  The main config file lives at
`/etc/opt/roxy/config.json`, while the MIME rules live at
`/etc/opt/roxy/mime.json`.

The overall layout of `config.json` looks like this:

```
{
  "global": {...},   # Object -----------> Section "global"
  "hosts": [...],    # Array of Strings -> Section "hosts"
  "targets": {...},  # Object -----------> Section "targets"
  "rules": [...]     # Array of Objects -> Section "rules"
}
```

All sections are _technically_ optional, but nearly every setup will
want to define [`"hosts"`](#section-hosts),
[`"targets"`](#section-targets), and [`"rules"`](#section-rules).

Here is an example Roxy configuration for `config.json` that demonstrates both
static content serving and reverse proxying, plus a few simple header
rewrites:

```json
{
  "global": {
    "mimeFile": "/etc/opt/roxy/mime.json",
    "storage": {
      "engine": "fs",
      "path": "/var/opt/roxy/lib/acme"
    }
  },
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

### Section `"global"`

Section `"global"` groups together miscellaneous configuration items that don't fit in any other category.

It contains the following fields and sub-sections:
* [`"mimeFile"`](#field-globalmimeFile)
* [`"etcd"`](#subsection-globaletcd)
* [`"zk"`](#subsection-globalzk)
* [`"storage"`](#subsection-globalstorage)
* [`"pages"`](#subsection-globalpages)

```
{
  "global": {
    "mimeFile": "...",  # String -> path to mime.json
    "etcd": {...},      # Object -> Subsection "global"."etcd"
    "zk": {...},        # Object -> Subsection "global"."zk"
    "storage": {...},   # Object -> Subsection "global"."storage"
    "pages": {...}      # Object -> Subsection "global"."pages"
  },
  ...
}
```

#### Field `"global"."mimeFile"`

Field `"global"."mimeFile"` specifies the path to the `mime.json` ancillary configuration file.
If empty or not specified, it defaults to `"/etc/opt/roxy/mime.json"`.

If the file does not exist, it is not a fatal error.  Instead, Roxy will fall back on its
built-in defaults, which should be identical to the `mime.json.example` file that ships with
Roxy.

The MIME file is in JSON format and contains an Array of Objects.  It has the following structure:

```
[
  ...
  {
    "suffixes": ["..."],       # Array of Strings, list of literal suffixes matched against the full path
    "contentType": "...",      # String, the "Content-Type" header (default `application/octet-stream`)
    "contentLanguage": "...",  # String, the "Content-Language" header (default absent)
    "contentEncoding": "..."   # String, the "Content-Encoding" header (default absent)
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

#### Subsection `"global"."etcd"`

Subsection `"global"."etcd"` enables the use of [Etcd](https://etcd.io/) to store TLS certificates
(see [subsection `"global"."storage"`](#subsection-globalstorage)) and to look up backends
(see [section `"targets"`](#section-targets)).  It has the following structure:

```
{
  "global": {
    ...
    "etcd": {
      "endpoints": ["..."],      # Array of strings, addresses of the etcd cluster members
      "tls": {...},              # TLS client configuration (optional; see below)
      "username": "...",         # etcd username (optional)
      "password": "...",         # etcd password (optional)
      "dialTimeout": "...",      # Duration string, e.g. "5s", max time to wait when connecting
      "keepAliveTime": "...",    # Duration string, time between sending of keep-alive requests
      "keepAliveTimeout": "..."  # Duration string, max time without receiving a keep-alive reply
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

#### Subsection `"global"."zk"`

Subsection `"global"."zk"` enables the use of [Apache ZooKeeper](https://zookeeper.apache.org/) to
store TLS certificates ([subsection `"global"."storage"`](#subsection-globalstorage)) and to look
up backends ([section `"targets"`](#section-targets)).  It has the following structure:

```
{
  "global": {
    ...
    "zk": {
      "servers": ["..."],       # Array of strings, addresses of the ZK cluster members
      "sessionTimeout": "...",  # Duration string, e.g. "30s", max time that the ZK cluster should keep our session alive if we get disconnected
      "auth": {
        "scheme": "digest",     # Other schemes are possible, see ZooKeeper docs
        "raw": "...",           # Base-64 encoded binary data; most users will use "username" and "password" instead
        "username": "...",      # String, the username to use
        "password": "..."       # String, the password to use
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

#### Subsection `"global"."storage"`

Subsection `"global"."storage"` determines where Roxy will store TLS certificates
obtained via the ACME protocol, as well as its long-lived private key for speaking
with the ACME server.  It has the following structure:

```
{
  "global": {
    ...
    "storage": {
      "engine": "...",  # String, one of "fs", "etcd", or "zk"
      "path": "..."     # String, meaning depends on which storage engine is in use
    },
    ...
  },
  ...
}
```

The `"path"` field is the name of a directory in the namespace of the given engine.

(NB: Etcd does not have "directories", per se; instead, `"path"` is suffixed with `"/"`
to form a search prefix.  This feels enough like a directory that the "path"
nomenclature still fits.)

The default, which takes effect **only** if there is no `"global"."storage"` sub-section at all, is:

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

#### Subsection `"global"."pages"`

Subsection `"global"."pages"` tells Roxy where to find your custom HTML templates for error pages,
redirects, and filesystem index pages.  It has the following structure:

```
{
  "global": {
    ...
    "pages": {
      "rootDir": "...",                 # String, path to the directory which contains template files
      "map": {...},                     # Object, used to customize individual error codes
      "defaultContentType": "...",      # String, sets the default value for the "Content-Type" header
      "defaultContentLanguage": "...",  # String, sets the default value for the "Content-Language" header
      "defaultContentEncoding": "..."   # String, sets the default value for the "Content-Encoding" header
    },
    ...
  },
  ...
}
```

The `"global"."pages"."map"` field is structured as:

```
{
  "global": {
    ...
    "pages": {
      ...
      "map": {
        ...
        "<error-code-or-special>": {  # String, 3-digit HTTP status code or one of: "4xx", "5xx", "redir", "index"
          "fileName": "...",          # String, relative path to HTML template file in Go "html/template" format
          "contentType": "...",       # String, value for the "Content-Type" header
          "contentLanguage": "...",   # String, value for the "Content-Language" header
          "contentEncoding": "..."    # String, value for the "Content-Encoding" header
        },
        ...
      },
      ...
    },
    ...
  },
  ...
}
```

***

### Section `"hosts"`

Section `"hosts"` is a list of host patterns.  A host pattern is a string, which matches one of the
following patterns: a domain name, a domain name prefixed with `"*."`, or the exact string `"*"`.

This controls which domains Roxy is willing to serve, and more importantly, which domains Roxy is
willing to obtain [Let's Encrypt](https://letsencrypt.org/) certificates for.

* A domain name, such as `"example.com"`, represents an exact match of that domain.

* A domain name prefixed with `"*."`, such as `"*.example.com"`, means a match of that domain or
any subdomains beneath it.

* A literal `"*"` means that any requested domain name is considered to match.
**WARNING: This should be considered a security risk.**
Someone who doesn't like one of your websites could potentially make HTTPS requests for
domains that you don't actually own, which will cause Let's Encrypt to block your webserver
from obtaining future TLS certs.

### Section `"targets"`

Section `"targets"` is a map from target names (strings) to target configurations (objects).
A target name is a unique identifier that will be used by the [`"rules"` section](#section-rules)
to refer back to the target configuration.

The target configuration has the following structure:

```
{
  ...
  "targets": {
    ...
    "target-name": {
      "type": "...",    # String, one of "fs", "http", or "grpc"
      "path": "...",    # String, "fs" only, path to the intended directory
      "target": "...",  # String, "http" and "grpc" only, resolve spec to reach the intended backend
      "tls": {...}      # Object, "http" and "grpc" only, TLS client configuration (optional, see below)
    },
    ...
  },
  ...
}
```

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

A target that uses <span title="mutual TLS"><em>mTLS</em></span> to connect to an HTTPS backend,
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

### Section `"rules"`

Section `"rules"` is an array of rules (objects).  A rule is an optional set of matching criteria,
an optional list of mutations to apply, and an optional target spec.

```
{
  ...
  "rules": [
    ...
    {
      "match": {...},      # Object, a map from header names (strings) to header value regexps (strings)
      "mutations": [...],  # Array of Objects, each of which specifies a mutation to apply to matching requests
      "target": "..."      # String, a target spec (described below)
    },
    ...
  ],
  ...
}
```

The `"match"` field consists of zero or more header matches, where each match is the header name (map key) and
the regular expression which the header value must match (map value).

**NB: the regexp is implicitly anchored with `^` and `$`, i.e. a full string match.**

The rule only applies if **all** header
matches have successfully matched against the *original, unmodified request*.  If any of the header matches
fails to match the request, then the current rule is ignored.

There are a few special header names:
* "Host" matches the request hostname
* "Method" matches the request HTTP method
* "Path" matches the path part of the request HTTP URI

The `"mutations"` field consists of zero or more mutations that are applied if (and only if) the request met
the requirements of the `"match"` field.  The general structure of a mutation object is as follows:

```
{
  ...
  "rules": [
    ...
    {
      ...
      "mutations": [
        ...
        {
          "type": "...",    # String, one of "request-host", "request-path", "request-query", "request-header", "response-header-pre", or "response-header-post"
          "header": "...",  # String, the name of the header (only for "request-header", "response-header-pre", or "response-header-post")
          "search": "...",  # String, a regexp to match against the desired field
          "replace": "..."  # String, a replacement string (see below)
        },
        ...
      ],
      ...
    },
    ...
  ],
  ...
}
```

As with the `"match"` field, the regexp in the `"search"` field is implicitly anchored
with `^` and `$`.

The replacement string is a template in
[Go `"text/template"` format](https://golang.org/pkg/text/template/), which is called with
a `.` of type `[]string`, representing the return value from calling
[`FindStringSubmatch`](https://golang.org/pkg/regexp/#Regexp.FindStringSubmatch)
on the `"search"` regexp.  As a convenience, the strings `\\0` through `\\9` are synonyms
for `{{ index . N }}`.

The `"target"` field consists of a target spec.

If the target spec is present at all, it means that **every request** which meets the
requirements of the `"match"` field will terminate with this rule.  No further rules
will be processed.  Conversely, if no target spec is present, then processing continues
to the next matching rule.

A target spec is normally the name of a target configuration in
[the `"targets"` section](#section-targets).  However, there are a few special patterns
which specify other behavior:

* `"ERROR:<status>"` causes Roxy to fail the request with the given 4xx or 5xx status code
* `"REDIR:<status>:<url>"` causes Roxy to send a redirect with the given 3xx status code and URL

The `<url>` in `REDIR:<status>:<url>` is a template in
[Go `"text/template"` format](https://golang.org/pkg/text/template/), which is called with
the current [`*url.URL`](https://golang.org/pkg/net/url/#URL) of the request
(as mutated by all matching rules up to this point).  This means that, if you want to rewrite
the URL path and then redirect the client in a single rule, you can do it with a mutation of
the form:

```json
{
  "rules": [
    {
      "match": {
        "path": "/foo/bar(/.*)?"
      },
      "mutations": [
        {
          "type": "request-path",
          "search": "/foo/bar(/.*)?",
          "replace": "/foo/baz\\1"
        }
      ],
      "target": "REDIR:302:{{.}}"
    }
  ]
}
```

***

### TLS client configuration

Some sections, such as [`"global"."etcd"`](#subsection-globaletcd) and
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
<span title="mutual TLS"><em>mTLS</em></span>, is as follows:

```
...
"tls": {},
...
```
