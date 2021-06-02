package generator

import (
	"github.com/exgphe/kin-openapi/routers"
	"net/http"

	"github.com/muonsoft/openapi-mock/internal/openapi/generator/content"
	"github.com/muonsoft/openapi-mock/internal/openapi/generator/data"
	"github.com/muonsoft/openapi-mock/internal/openapi/generator/negotiator"
)

type ResponseGenerator interface {
	GenerateResponse(request *http.Request, route *routers.Route) (*Response, error)
	GenerateRequestData(request *http.Request, route *routers.Route) (interface{}, error)
}

func New(dataGenerator data.MediaGenerator) ResponseGenerator {
	return &coordinatingGenerator{
		contentTypeNegotiator: negotiator.NewContentTypeNegotiator(),
		statusCodeNegotiator:  negotiator.NewStatusCodeNegotiator(),
		contentGenerator:      content.NewGenerator(dataGenerator),
	}
}
