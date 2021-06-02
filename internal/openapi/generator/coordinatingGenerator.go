package generator

import (
	"github.com/exgphe/kin-openapi/routers"
	"net/http"

	"github.com/muonsoft/openapi-mock/internal/openapi/generator/content"
	"github.com/muonsoft/openapi-mock/internal/openapi/generator/negotiator"
	"github.com/pkg/errors"
)

type coordinatingGenerator struct {
	statusCodeNegotiator  negotiator.StatusCodeNegotiator
	contentTypeNegotiator negotiator.ContentTypeNegotiator
	contentGenerator      content.Generator
}

func (generator *coordinatingGenerator) GenerateResponse(request *http.Request, route *routers.Route) (*Response, error) {
	responseKey, statusCode, err := generator.statusCodeNegotiator.NegotiateStatusCode(request, route.Operation.Responses)
	if err != nil {
		return nil, errors.WithMessage(err, "[coordinatingGenerator] failed to negotiate response")
	}
	bestResponse := route.Operation.Responses[responseKey].Value
	contentType := generator.contentTypeNegotiator.NegotiateContentType(request, bestResponse)

	contentData, err := generator.contentGenerator.GenerateContent(request.Context(), bestResponse, contentType)
	if err != nil {
		return nil, errors.WithMessage(err, "[coordinatingGenerator] failed to generate response data")
	}

	response := &Response{
		StatusCode:  statusCode,
		ContentType: contentType,
		Data:        contentData,
	}

	return response, nil
}

func (generator *coordinatingGenerator) GenerateRequestData(request *http.Request, route *routers.Route) (interface{}, error) {
	bestRequest := route.Operation.RequestBody.Value
	return generator.contentGenerator.(*content.DelegatingGenerator).Matchers[0].Generator.(*content.MediaGenerator).ContentGenerator.GenerateData(request.Context(), bestRequest.GetMediaType("application/yang-data+json"))
}
