package content

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/exgphe/kin-openapi/openapi3"
	apperrors "github.com/muonsoft/openapi-mock/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestDelegatingGenerator_GenerateContent_MatchingProcessorFound_ResponseProcessedByMatchingProcessor(t *testing.T) {
	matchingGenerator := &MockGenerator{}
	generator := &DelegatingGenerator{
		Matchers: []ContentMatcher{
			{
				Pattern:   regexp.MustCompile("^application/.*json$"),
				Generator: matchingGenerator,
			},
		},
	}
	contentType := "application/any-json"
	response := &openapi3.Response{}
	matchingGenerator.On("GenerateContent", mock.Anything, response, contentType).Return("data", nil).Once()

	content, err := generator.GenerateContent(context.Background(), response, contentType)

	matchingGenerator.AssertExpectations(t)
	assert.NoError(t, err)
	assert.Equal(t, "data", content)
}

func TestDelegatingGenerator_GenerateContent_NoMatchingProcessorFound_MediaTypeAndError(t *testing.T) {
	generator := &DelegatingGenerator{
		Matchers: []ContentMatcher{},
	}
	contentType := "contentType"
	response := &openapi3.Response{}

	content, err := generator.GenerateContent(context.Background(), response, contentType)

	assert.EqualError(t, err, "generating response for content type 'contentType' is not supported")
	var notSupported *apperrors.NotSupported
	assert.True(t, errors.As(err, &notSupported))
	assert.Nil(t, content)
}

func TestDelegatingGenerator_GenerateContent_NoContentType_EmptyString(t *testing.T) {
	generator := &DelegatingGenerator{
		Matchers: []ContentMatcher{},
	}
	response := &openapi3.Response{}

	content, err := generator.GenerateContent(context.Background(), response, "")

	assert.NoError(t, err)
	assert.Equal(t, "", content)
}
