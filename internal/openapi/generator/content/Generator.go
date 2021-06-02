package content

import (
	"context"
	"regexp"

	"github.com/exgphe/kin-openapi/openapi3"
	"github.com/muonsoft/openapi-mock/internal/openapi/generator/data"
)

type Generator interface {
	GenerateContent(ctx context.Context, response *openapi3.Response, contentType string) (interface{}, error)
}

func NewGenerator(generator data.MediaGenerator) Generator {
	mediaGenerator := &MediaGenerator{ContentGenerator: generator}

	return &DelegatingGenerator{
		Matchers: []ContentMatcher{
			{
				Pattern:   regexp.MustCompile("^application/.*json$"),
				Generator: mediaGenerator,
			},
			{
				Pattern:   regexp.MustCompile("^application/.*xml$"),
				Generator: mediaGenerator,
			},
			{
				Pattern:   regexp.MustCompile("^text/html$"),
				Generator: &htmlGenerator{contentGenerator: generator},
			},
			{
				Pattern:   regexp.MustCompile("^text/plain$"),
				Generator: &plainTextGenerator{contentGenerator: generator},
			},
		},
	}
}
