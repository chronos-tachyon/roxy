# Roxy the Frontend Proxy

[![Go](https://img.shields.io/github/go-mod/go-version/chronos-tachyon/roxy)](https://golang.org/)
[![License](https://img.shields.io/badge/license-Hippocratic%202.1-brightgreen)](https://firstdonoharm.dev/version/2/1/license/)
[![Build Status](https://img.shields.io/github/workflow/status/chronos-tachyon/roxy/Go)](https://github.com/chronos-tachyon/roxy/actions/workflows/go.yml)
[![Latest Release](https://img.shields.io/github/v/release/chronos-tachyon/roxy?sort=semver)](https://github.com/chronos-tachyon/roxy/releases)
[![Docker Pulls](https://img.shields.io/docker/pulls/chronostachyon/roxy)](https://hub.docker.com/r/chronostachyon/roxy)

> ![Roxy Lalonde](https://chronos-tachyon.net/img/roxy-lalonde.png)
> 
> Our mascot, [Roxy Lalonde](https://mspaintadventures.fandom.com/wiki/Roxy_Lalonde).

Roxy is an Internet-facing HTTPS frontend proxy that's meant to scale from
hobbyist websites to very large installations, assuming that you're a fan of
[Let's Encrypt](https://letsencrypt.org/) or other ACME providers for issuing
your TLS certificates.

Because Roxy's uses of HTTPS and ACME are not optional, Roxy _only_ supports
being an Internet-facing webserver.  It does one thing, and does it well.
There is basic support for serving static content directly from the local
filesystem, but Roxy does not support, and will never support, scripting
engines in the style of PHP, `mod_python`, and so on.  Instead, users of
Roxy are expected to use Roxy as a reverse proxy, running their dynamic
content on "micro-frontends" _behind_ Roxy.  Roxy will take care of all the
Internet-facing stuff that you would normally have to re-implement yourself,
such as TLS certificates and modern security-hardening HTTP headers.

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

## More documentation

See also:
* The [Full Configuration Reference](configuration.html)
* [Frequently Asked Questions](faq.html)

