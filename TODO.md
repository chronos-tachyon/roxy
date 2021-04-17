Things that I would like to do with Roxy:

* [x] Support etcd and zookeeper for ACME cert/key/challenge storage
* [ ] X-Forwarded-IP et al for backend requests
* [ ] Fork of io.Copy that respects ctx cancellation
* [ ] Fork of http.ServeContent that respects ctx cancellation
* [ ] Make fs target respond to OPTIONS
* [ ] Make fs target reject POST et al with Method Not Allowed
* [x] Don't try to generate checksums/etag when file is larger than `$num` MiB
* [x] Unique request IDs for log correlation
* [x] Re-open log file on SIGHUP
* [ ] Make journald logging more useful
* [ ] Scripts to generate Debian package
* [ ] Scripts to generate Docker image
* [ ] Prometheus metrics on a new HTTP listen port
* [x] WrappedWriter.Error should not Write after WriteHeader if r.Method is HEAD
* [x] Recognize symlinks in FileSystemHandler.ServeDir
* [ ] User control of in-memory cache threshold
* [ ] Investigate possible bidirectional support of HTTP trailers to/from backends
* [ ] Investigate possible use of HTTP body filters to/from backends
* [ ] Optional TLS/mTLS between Roxy and backends
* [ ] Resolve backend IPs through DNS A + port, DNS SRV, [Finagle ServerSets in ZooKeeper](http://stevenskelton.ca/finagle-serverset-clusters-using-zookeeper/), others?
