package content

import (
	"context"

	"github.com/exgphe/kin-openapi/openapi3"
	"github.com/muonsoft/openapi-mock/internal/openapi/generator/data"
)

type MediaGenerator struct {
	ContentGenerator data.MediaGenerator
}

func (generator *MediaGenerator) GenerateContent(ctx context.Context, response *openapi3.Response, contentType string) (interface{}, error) {
	mediaType := response.Content[contentType]
	if mediaType == nil {
		return "", nil
	}

	return generator.ContentGenerator.GenerateData(ctx, mediaType)
}
