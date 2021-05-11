# roxy

[![Go](https://img.shields.io/github/go-mod/go-version/chronos-tachyon/roxy?logo=go)](https://golang.org/)
[![Latest Release](https://img.shields.io/github/v/release/chronos-tachyon/roxy?logo=github&sort=semver)](https://github.com/chronos-tachyon/roxy/releases)
[![License](https://img.shields.io/github/license/chronos-tachyon/roxy?logo=apache)](https://opensource.org/licenses/Apache-2.0)
[![Build Status](https://img.shields.io/github/workflow/status/chronos-tachyon/roxy/Go?logo=github-actions)](https://github.com/chronos-tachyon/roxy/actions/workflows/go.yml)
[![Docker Pulls](https://img.shields.io/docker/pulls/chronostachyon/roxy?logo=docker)](https://hub.docker.com/r/chronostachyon/roxy)
[![Go Report Card](https://goreportcard.com/badge/github.com/chronos-tachyon/roxy)](https://goreportcard.com/report/github.com/chronos-tachyon/roxy)

**Roxy the Frontend Proxy**

> ![Roxy Lalonde](https://chronos-tachyon.net/img/roxy-lalonde.png)
> 
> Our mascot, [Roxy Lalonde](https://mspaintadventures.fandom.com/wiki/Roxy_Lalonde).

Roxy is an Internet-facing frontend proxy which provides the following
features:

* Automatically obtains TLS certificates from Let's Encrypt; no need to
  manage certificates manually or to install and configure Certbot
* Able to inject and rewrite headers in the incoming request, before your
  application sees them
* Able to inject and rewrite headers in the outgoing response, allowing you to
  centrally control your Internet-visible server headers
* Comprehensive request logging in [JSON Lines](https://jsonlines.org/) format
* Can serve static files directly, without need for nginx, lighttpd, etc.

See [our GitHub Pages site](https://chronos-tachyon.github.io/roxy/) for more
documentation.
