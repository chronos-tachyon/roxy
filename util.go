package main

import (
	"net/http"
	"strings"
)

func simplifyHTTPMethod(str string) string {
	switch str {
	case http.MethodOptions:
		return http.MethodOptions
	case http.MethodGet:
		return http.MethodGet
	case http.MethodHead:
		return http.MethodHead
	case http.MethodPost:
		return http.MethodPost
	case http.MethodPut:
		return http.MethodPut
	case http.MethodPatch:
		return http.MethodPatch
	case http.MethodDelete:
		return http.MethodDelete
	default:
		return "OTHER"
	}
}

func simplifyHTTPStatusCode(statusCode int) string {
	switch statusCode {
	case 100:
		return "100"
	case 101:
		return "101"
	case 0, 200:
		return "200"
	case 204:
		return "204"
	case 206:
		return "206"
	case 301:
		return "301"
	case 302:
		return "302"
	case 304:
		return "304"
	case 307:
		return "307"
	case 308:
		return "308"
	case 400:
		return "400"
	case 401:
		return "401"
	case 403:
		return "403"
	case 404:
		return "404"
	case 405:
		return "405"
	case 500:
		return "500"
	case 503:
		return "503"
	}

	switch {
	case statusCode < 200:
		return "1xx"
	case statusCode < 300:
		return "2xx"
	case statusCode < 400:
		return "3xx"
	case statusCode < 500:
		return "4xx"
	default:
		return "5xx"
	}
}

func simplifyTargetKey(key string) string {
	switch {
	case strings.HasPrefix(key, "ERROR:"):
		return "!ERROR"
	case strings.HasPrefix(key, "REDIR:"):
		return "!REDIR"
	default:
		return key
	}
}
