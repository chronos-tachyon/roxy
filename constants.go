package main

const (
	xattrMimeType  = "user.mimetype"
	xattrMimeLang  = "user.mimelang"
	xattrMd5sum    = "user.md5sum"
	xattrSha1sum   = "user.sha1sum"
	xattrSha256sum = "user.sha256sum"
	xattrEtag      = "user.etag"
)

const defaultIndexPageType = "text/html; charset=utf-8"
const defaultIndexPageLang = "en"
const defaultIndexPageTemplate = `<!DOCTYPE html>
<html>
	<head>
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
