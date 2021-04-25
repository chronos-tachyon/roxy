# roxy

[![Go](https://img.shields.io/github/go-mod/go-version/chronos-tachyon/roxy)](https://golang.org/)
[![License](https://img.shields.io/badge/license-Hippocratic%202.1-brightgreen)](https://firstdonoharm.dev/version/2/1/license/)
[![Build Status](https://img.shields.io/github/workflow/status/chronos-tachyon/roxy/Go)](https://github.com/chronos-tachyon/roxy/actions/workflows/go.yml)

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
* Comprehensive request logging in JSON Lines format
* Can serve static files directly, without need for nginx, lighttpd, etc.
