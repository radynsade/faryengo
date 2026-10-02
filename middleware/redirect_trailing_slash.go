package middleware

import (
	"net/http"
	"path"
)

func RedirectTrailigSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		targetPath := path.Clean(request.URL.Path)

		if request.URL.Path != targetPath {
			url := *request.URL
			url.Path = targetPath
			http.Redirect(writer, request, url.String(), http.StatusMovedPermanently)
		} else {
			next.ServeHTTP(writer, request)
		}
	})
}
