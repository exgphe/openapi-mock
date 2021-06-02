package data

import (
	"context"

	"github.com/exgphe/kin-openapi/openapi3"
	"github.com/pkg/errors"
)

type textGenerator struct {
	generator *rangedTextGenerator
}

func (generator *textGenerator) GenerateDataBySchema(ctx context.Context, schema *openapi3.Schema) (Data, error) {
	var maxLength uint64
	if schema.MinLength < defaultMaxLength {
		maxLength = defaultMaxLength
	} else {
		maxLength = schema.MinLength + defaultMaxLength
	}

	if schema.MaxLength != nil && maxLength > *schema.MaxLength {
		maxLength = *schema.MaxLength
	}

	if maxLength < schema.MinLength {
		return "", errors.WithStack(&ErrGenerationFailed{
			GeneratorID: "textGenerator",
			Message:     "max length cannot be less than min length",
		})
	}

	return generator.generator.generateRangedText(int(schema.MinLength), int(maxLength)), nil
}
