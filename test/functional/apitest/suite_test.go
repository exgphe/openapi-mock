package apitest

import (
	"net/http"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/muonsoft/openapi-mock/internal/application/config"
	"github.com/muonsoft/openapi-mock/internal/application/di"
	"github.com/stretchr/testify/suite"
)

type APISuite struct {
	suite.Suite
}

func TestApi(t *testing.T) {
	suite.Run(t, new(APISuite))
}

func (suite *APISuite) createOpenAPIHandler(configuration config.Configuration) http.Handler {
	configuration.ResponseTimeout = time.Second
	specificationPath := "./../../resources/openapi-files/" + configuration.SpecificationURL
	factory := di.NewFactory(&configuration)
	specificationLoader := factory.CreateSpecificationLoader()
	specification, err := specificationLoader.LoadFromURI(specificationPath)
	if err != nil {
		suite.T().Fatal("failed to load specification:", err)
	}
	router, err := legacy.NewRouter(specification)
	if err != nil {
		suite.T().Fatal("failed to set up routing:", err)
	}

	return factory.CreateHTTPHandler(router)
}
