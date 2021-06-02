package data

import (
	"context"

	"github.com/exgphe/kin-openapi/openapi3"
)

type CoordinatingMediaGenerator struct {
	useExamples     UseExamplesEnum
	schemaGenerator schemaGenerator
}

func (generator *CoordinatingMediaGenerator) GenerateData(ctx context.Context, mediaType *openapi3.MediaType) (Data, error) {
	if generator.useExamples == IfPresent || generator.useExamples == Exclusively {
		if mediaType.Example != nil {
			return mediaType.Example, nil
		}
		if mediaType.Examples != nil {
			for _, example := range mediaType.Examples {
				return example.Value.Value, nil
			}
		}
	}

	if generator.useExamples == Exclusively {
		return nil, nil
	}

	return generator.schemaGenerator.GenerateDataBySchema(ctx, mediaType.Schema.Value)
}
