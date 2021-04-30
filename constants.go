package main

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

const defaultIndexPageTemplate = `<!DOCTYPE html>
<html>
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>Listing of {{.Path}}</title>
	</head>
	<body>
		<h1>Listing of {{.Path}}</h1>
		<pre>
{{- with $root := . -}}
{{- range $entry := .Entries -}}
{{$entry.Mode}} {{printf "%*d" $root.NLinkWidth $entry.NLink}} {{printf "%*s" (neg $root.OwnerWidth) $entry.Owner}} {{printf "%*s" (neg $root.GroupWidth) $entry.Group}} {{printf "%*d" $root.SizeWidth $entry.Size}} {{$entry.MTime.UTC.Format "2006-01-02 15:04"}} <a href="{{$entry.Name}}{{$entry.Slash}}">{{$entry.Name}}{{$entry.Slash}}</a>{{pad (sub $root.NameWidth (add (uint (runelen $entry.Name)) (uint (len $entry.Slash))))}} {{if $entry.IsLink}}→ {{$entry.Link}}{{else}}[{{printf "%*s" (neg $root.ContentTypeWidth) $entry.ContentType}}] [{{printf "%*s" (neg $root.ContentLangWidth) $entry.ContentLang}}]{{end}}
{{end -}}
{{- end -}}
		</pre>
	</body>
</html>
`

const defaultRedirPageTemplate = `<!DOCTYPE html>
<html>
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>{{.StatusLine}}</title>
	</head>
	<body>
		<h1>{{.StatusLine}}</h1>
		<h2><a href="{{.URL}}">{{.URL}}</a></h2>
	</body>
</html>
`

const defaultErrorPageTemplate = `<!DOCTYPE html>
<html>
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>{{.StatusLine}}</title>
	</head>
	<body>
		<h1>{{.StatusLine}}</h1>
	</body>
</html>
`

const defaultMimeFileJSON = `
[
  {
    "suffixes": [
      ".txt",
      ".text",
      ".asc",
      "/README",
      "/LICENSE",
      "/COPYING",
      "/InRelease",
      "/Index",
      "/Packages",
      "/Release",
      "/Sources"
    ],
    "contentType": "text/plain; charset=utf-8",
    "contentLanguage": "en-US"
  },
  {
    "suffixes": [".html", ".htm", ".shtml"],
    "contentType": "text/html; charset=utf-8",
    "contentLanguage": "en-US"
  },
  {
    "suffixes": [".md", ".markdown"],
    "contentType": "text/markdown; charset=utf-8",
    "contentLanguage": "en-US"
  },
  {
    "suffixes": [".js"],
    "contentType": "text/javascript; charset=utf-8"
  },
  {
    "suffixes": [".js.map"],
    "contentType": "application/json"
  },
  {
    "suffixes": [".css"],
    "contentType": "text/css; charset=utf-8"
  },
  {
    "suffixes": [".csv"],
    "contentType": "text/csv; charset=utf-8"
  },
  {
    "suffixes": [".png"],
    "contentType": "image/png"
  },
  {
    "suffixes": [".gif"],
    "contentType": "image/gif"
  },
  {
    "suffixes": [".jpeg", ".jpg", ".jpe"],
    "contentType": "image/jpeg"
  },
  {
    "suffixes": [".svg", ".svgz"],
    "contentType": "image/svg+xml"
  },
  {
    "suffixes": [".webp"],
    "contentType": "image/webp"
  },
  {
    "suffixes": [".ico"],
    "contentType": "image/vnd.microsoft.icon"
  },
  {
    "suffixes": [".wav"],
    "contentType": "audio/wave"
  },
  {
    "suffixes": [".ogg", ".oga", ".opus", ".spx"],
    "contentType": "audio/ogg"
  },
  {
    "suffixes": [".flac"],
    "contentType": "audio/flac"
  },
  {
    "suffixes": [".mpega", ".mpga", ".mp2", ".mp3", ".m4a"],
    "contentType": "audio/mpeg"
  },
  {
    "suffixes": [".m3u"],
    "contentType": "audio/mpegurl"
  },
  {
    "suffixes": [".pls"],
    "contentType": "audio/x-scpls"
  },
  {
    "suffixes": [".mid", ".midi"],
    "contentType": "audio/midi"
  },
  {
    "suffixes": [".ogv"],
    "contentType": "video/ogg"
  },
  {
    "suffixes": [".mkv"],
    "contentType": "video/x-matroska"
  },
  {
    "suffixes": [".mpeg", ".mpg", ".mpe"],
    "contentType": "video/mpeg"
  },
  {
    "suffixes": [".mp4", ".m4v"],
    "contentType": "video/mp4"
  },
  {
    "suffixes": [".mov", ".qt"],
    "contentType": "video/quicktime"
  },
  {
    "suffixes": [".mng"],
    "contentType": "video/mng"
  },
  {
    "suffixes": [".webm"],
    "contentType": "video/webm"
  },
  {
    "suffixes": [".flv"],
    "contentType": "video/x-flv"
  },
  {
    "suffixes": [".avi"],
    "contentType": "video/x-msvideo"
  },
  {
    "suffixes": [".asf"],
    "contentType": "video/x-ms-asf"
  },
  {
    "suffixes": [".wmv"],
    "contentType": "video/x-ms-wmv"
  },
  {
    "suffixes": [".xml", ".xsd"],
    "contentType": "application/xml"
  },
  {
    "suffixes": [".xsl", ".xslt"],
    "contentType": "application/xslt+xml"
  },
  {
    "suffixes": [".rdf"],
    "contentType": "application/rdf+xml"
  },
  {
    "suffixes": [".rss"],
    "contentType": "application/rss+xml"
  },
  {
    "suffixes": [".atom"],
    "contentType": "application/atom+xml"
  },
  {
    "suffixes": [".atomcat"],
    "contentType": "application/atomcat+xml"
  },
  {
    "suffixes": [".atomserv"],
    "contentType": "application/atomserv+xml"
  },
  {
    "suffixes": [".json"],
    "contentType": "application/json"
  },
  {
    "suffixes": [".ogx"],
    "contentType": "application/ogg"
  },
  {
    "suffixes": [".pdf"],
    "contentType": "application/pdf"
  },
  {
    "suffixes": [".ttf", ".otf"],
    "contentType": "application/font-sfnt"
  },
  {
    "suffixes": [".woff"],
    "contentType": "application/font-woff"
  },
  {
    "suffixes": [".pgp", ".gpg"],
    "contentType": "application/pgp-encrypted"
  },
  {
    "suffixes": [".key"],
    "contentType": "application/pgp-keys"
  },
  {
    "suffixes": [".sig"],
    "contentType": "application/pgp-signature"
  },
  {
    "suffixes": [".zip"],
    "contentType": "application/zip"
  },
  {
    "suffixes": [".7z"],
    "contentType": "application/x-7z-compressed"
  },
  {
    "suffixes": [".tar", ".tgz", ".tar.gz", ".tar.bz2", ".tar.xz"],
    "contentType": "application/x-tar"
  },
  {
    "suffixes": [".jar"],
    "contentType": "application/java-archive"
  },
  {
    "suffixes": [".deb", ".ddeb", ".udeb"],
    "contentType": "application/vnd.debian.binary-package"
  },
  {
    "suffixes": [".gz"],
    "contentType": "application/gzip"
  },
  {
    "suffixes": [".xz"],
    "contentType": "application/x-xz"
  }
]
`
