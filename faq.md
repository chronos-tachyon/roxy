![Roxy logo](https://chronos-tachyon.net/img/roxy-tshirt.png)

# Roxy: Frequently Asked Questions

## Q: Does Roxy scale horizontally?

Yes!  Roxy defaults to storing ACME client keys, challenges, and certificates
on local disk, but you can also use a [ZooKeeper](https://zookeeper.apache.org/)
or [Etcd](https://etcd.io/) cluster as a distributed storage backend.

At the moment, the configuration file is still delivered via local disk only,
but obtaining the bulk of the configuration from the storage backend is a
future feature under consideration.

## Q: Why use Roxy instead of competing reverse proxies like Caddy?

Roxy has a laser focus on its core use case, and does not support modules,
plugins, or other abstractions that make Caddy's configuration a little
baroque and tricky to understand.  Roxy's simplicity allows the configuration
file to be simple and easy to understand.

Use Caddy if you already know Caddy, or if you need Caddy's flexibility.
Use Roxy if Roxy fits your use case and you want Roxy's simplicity.
