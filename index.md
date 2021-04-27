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

## Configuring Roxy

Roxy is configured using JSON format.  The main config file lives at
`/etc/opt/roxy/config.json`, while the MIME rules live at
`/etc/opt/roxy/mime.json`.

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
