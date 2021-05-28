package handler

import (
	"encoding/json"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/muonsoft/openapi-mock/internal/openapi/generator"
	"github.com/muonsoft/openapi-mock/internal/openapi/responder"
	"github.com/muonsoft/openapi-mock/pkg/logcontext"
	"github.com/pkg/errors"
	"io"
	"io/fs"
	"io/ioutil"
	"net/http"
	"os"
)

type responseGeneratorHandler struct {
	router            *openapi3filter.Router
	responseGenerator generator.ResponseGenerator
	responder         responder.Responder
}

func NewResponseGeneratorHandler(
	router *openapi3filter.Router,
	responseGenerator generator.ResponseGenerator,
	responder responder.Responder,
) http.Handler {
	generatorHandler := &responseGeneratorHandler{
		router:            router,
		responseGenerator: responseGenerator,
		responder:         responder,
	}

	return &optionsHandler{
		router:      router,
		nextHandler: generatorHandler,
	}
}

func (handler *responseGeneratorHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	ctx := request.Context()
	logger := logcontext.LoggerFromContext(ctx)

	route, pathParameters, err := handler.router.FindRoute(request.Method, request.URL)

	if err != nil {
		http.NotFound(writer, request)

		logger.Debugf("Route '%s %s' was not found", request.Method, request.URL)
		return
	}

	routingValidation := &openapi3filter.RequestValidationInput{
		Request:    request,
		PathParams: pathParameters,
		Route:      route,
		Options: &openapi3filter.Options{
			ExcludeRequestBody: true,
		},
	}

	err = openapi3filter.ValidateRequest(ctx, routingValidation)
	var requestError *openapi3filter.RequestError
	if errors.As(err, &requestError) {
		http.NotFound(writer, request)
		logger.Infof("Route '%s %s' does not pass validation: %v", request.Method, request.URL, err.Error())

		return
	}

	if request.Body != http.NoBody && request.Body != nil {
		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				logger.Errorf("Cannot close body", err)
			}
		}(request.Body)
		bodyData, err := ioutil.ReadAll(request.Body)
		if err == nil {
			filename := "requests.json"
			requests := map[string][]interface{}{}
			_, err := os.Stat(filename)
			if !os.IsNotExist(err) {
				// file exists
				fileContent, _ := ioutil.ReadFile(filename)
				err := json.Unmarshal(fileContent, &requests)
				if err != nil {
					logger.Errorf("JSON read error", err)
				}
			}
			requestsOfPath, ok := requests[request.URL.Path]
			var dataJson map[string]interface{}
			err = json.Unmarshal(bodyData, &dataJson)
			if err != nil {
				logger.Errorf("Json unmarshal error", err)
			}
			if ok {
				requests[request.URL.Path] = append(requestsOfPath, dataJson)
			} else {
				requests[request.URL.Path] = []interface{}{dataJson}
			}
			fileData, err := json.Marshal(requests)
			if err != nil {
				logger.Errorf("Json marshal error", err)
			}
			err = ioutil.WriteFile(filename, fileData, fs.ModePerm)
			if err != nil {
				logger.Errorf("Cannot write file %s", filename, err)
			}
		} else {
			logger.Errorf("Cannot read body", err)
		}
	}

	response, err := handler.responseGenerator.GenerateResponse(request, route)
	if err != nil {
		handler.responder.WriteError(ctx, writer, err)
		return
	}

	handler.responder.WriteResponse(ctx, writer, response)
}
