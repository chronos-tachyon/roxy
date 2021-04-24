## Roxy the Frontend Proxy

> ![Roxy Lalonde](https://chronos-tachyon.net/img/roxy-lalonde.png)
> 
> Our mascot, [Roxy Lalonde](https://mspaintadventures.fandom.com/wiki/Roxy_Lalonde).

Roxy is an Internet-facing HTTPS frontend proxy that's meant to scale from hobbyist websites to
very large installations, assuming that you're a fan of [Let's Encrypt](https://letsencrypt.org/)
for your TLS certificates.

Roxy supports basic serving of files directly from the filesystem, so that you don't need
to also install Apache, lighttpd, nginx, or some other HTTP server just to serve your
static content.  It does not support, and will never support, scripting engines in the style
of PHP, `mod_python`, etc.  Instead, you are invited to run your web application as its own
webserver _behind_ Roxy, and Roxy will take care of all the Internet-facing stuff that you
would normally have to re-implement yourself, such as TLS certificates and modern
security-hardening HTTP headers.

Roxy currently supports HTTP, HTTPS, and gRPC (over both TLS and plaintext) to communicate
with backend web servers.
