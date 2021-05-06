package main

import (
	_ "embed"
)

const (
	xattrMimeType  = "user.mimetype"
	xattrMimeLang  = "user.mimelang"
	xattrMimeEnc   = "user.mimeenc"
	xattrMd5sum    = "user.md5sum"
	xattrSha1sum   = "user.sha1sum"
	xattrSha256sum = "user.sha256sum"
	xattrEtag      = "user.etag"

	defaultContentType = "text/html; charset=utf-8"
	defaultContentLang = "en"
	defaultContentEnc  = ""

	defaultConfigFile    = "/etc/opt/roxy/config.json"
	defaultMimeFile      = "/etc/opt/roxy/mime.json"
	defaultStorageEngine = "fs"
	defaultStoragePath   = "/var/opt/roxy/lib/acme"

	defaultMaxCacheSize         = 64 << 10 // 64 KiB
	defaultMaxComputeDigestSize = 4 << 20  // 4 MiB
)

//go:embed templates/index.html
var defaultIndexPageTemplate string

//go:embed templates/redir.html
var defaultRedirPageTemplate string

//go:embed templates/error.html
var defaultErrorPageTemplate string

//go:embed mime.json.example
var defaultMimeFileJSON string
