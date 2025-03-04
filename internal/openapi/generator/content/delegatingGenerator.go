package content

import (
	"context"
	"fmt"
	"regexp"

	"errors"
	"github.com/exgphe/kin-openapi/openapi3"
)

type DelegatingGenerator struct {
	Matchers []ContentMatcher
}

type ContentMatcher struct {
	Pattern   *regexp.Regexp
	Generator Generator
}

func (processor *DelegatingGenerator) GenerateContent(ctx context.Context, response *openapi3.Response, contentType string) (interface{}, error) {
	if contentType == "" {
		return "", nil
	}

	for _, matcher := range processor.Matchers {
		if matcher.Pattern.MatchString(contentType) {
			return matcher.Generator.GenerateContent(ctx, response, contentType)
		}
	}

	return nil, errors.New(fmt.Sprintf("generating response for content type '%s' is not supported", contentType))
}
