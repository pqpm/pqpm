package web

import (
	"embed"
	"html"
	"io/fs"
	"strings"
)

//go:embed static/*
var staticRoot embed.FS

//go:embed templates/index.html templates/login.html
var templatesRoot embed.FS

func StaticFS() fs.FS {
	sub, err := fs.Sub(staticRoot, "static")
	if err != nil {
		panic(err)
	}
	return sub
}

func IndexHTML() []byte {
	b, err := templatesRoot.ReadFile("templates/index.html")
	if err != nil {
		panic(err)
	}
	return b
}

func LoginHTML(errMsg string) []byte {
	b, err := templatesRoot.ReadFile("templates/login.html")
	if err != nil {
		panic(err)
	}
	page := string(b)
	if errMsg != "" {
		page = strings.Replace(page, "<!--ERROR-->", `<p class="err" role="alert">`+html.EscapeString(errMsg)+`</p>`, 1)
	} else {
		page = strings.Replace(page, "<!--ERROR-->", "", 1)
	}
	return []byte(page)
}
