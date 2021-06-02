package loader

import (
	"github.com/exgphe/kin-openapi/openapi3"
	"net/url"
)

type externalLoader interface {
	LoadFromURI(location *url.URL) (*openapi3.T, error)
	LoadFromFile(path string) (*openapi3.T, error)
}
