package handler

import (
	"encoding/json"
	"github.com/exgphe/kin-openapi/openapi3filter"
	"github.com/exgphe/kin-openapi/routers"
	"github.com/muonsoft/openapi-mock/database"
	"github.com/muonsoft/openapi-mock/internal/openapi/generator"
	"github.com/muonsoft/openapi-mock/internal/openapi/responder"
	"github.com/muonsoft/openapi-mock/pkg/logcontext"
	"github.com/pkg/errors"
	"github.com/spyzhov/ajson"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
)

type responseGeneratorHandler struct {
	router            *routers.Router
	responseGenerator generator.ResponseGenerator
	responder         responder.Responder
}

func NewResponseGeneratorHandler(
	router *routers.Router,
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

	route, pathParameters, err := (*handler.router).FindRoute(request)

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

	filename := "requests.json"
	db, err := database.Load(filename)
	if err != nil {
		logger.Errorf("Json read error", err)
		db = database.Database{Content: ajson.ObjectNode("", make(map[string]*ajson.Node))}
	}

	defer func() {
		err = db.Save(filename)
		if err != nil {
			logger.Errorf("Save requests error", err)
		}
	}()

	keyPath := database.RestconfPathToKeyPath(request.URL.Path)

	if request.Method != "OPTIONS" && request.Body != http.NoBody && request.Body != nil && !strings.Contains(request.URL.Path, "restconf/operations/") {
		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				logger.Errorf("Cannot close body", err)
			}
		}(request.Body)
		bodyData, err := ioutil.ReadAll(request.Body)
		if err != nil {
			logger.Errorf("Cannot read body", err)
		} else {
			body, err := ajson.Unmarshal(bodyData)
			if err == nil {
				switch request.Method {
				case "POST":
					bodyObject, err := body.GetObject()
					if err != nil {
						logger.Errorf("Body is not an object", err)
					} else {
						var underlyingNode *ajson.Node
						for key := range bodyObject {
							underlyingNode = bodyObject[key]
						}
						if underlyingNode == nil {
							logger.Errorf("body is empty", err)
						} else {
							if underlyingNode.IsArray() {
								arr, _ := underlyingNode.GetArray()
								err = db.SetArrayNode(keyPath, arr)
							} else {
								obj, _ := underlyingNode.GetObject()
								err = db.SetObjectNode(keyPath, obj)
							}
							if err != nil {
								logger.Errorf("Cannot Set Node", err)
							}
						}
					}
				case "PUT":
					underlyingNode, err := body.GetKey(body.Keys()[0])
					if err != nil {
						logger.Errorf("Cannot extract underlying node", err)
					} else {
						err := db.AppendNode(keyPath, underlyingNode)
						if err != nil {
							logger.Errorf("Cannot Append Body", err)
						}
					}
				default:
					logger.Errorf("Should not happen")
					break
				}
			} else {
				logger.Errorf("Cannot extract body", err)
			}
		}
	}

	response, err := handler.responseGenerator.GenerateResponse(request, route)
	if err != nil {
		handler.responder.WriteError(ctx, writer, err)
		return
	}
	// Try to read from database
	if request.Method == "GET" && !strings.Contains(request.URL.Path, "restconf/operations/") {
		entry, err := db.Get(keyPath)
		if err == nil {
			err = json.Unmarshal(entry.Source(), &response.Data)
			if err != nil {
				logger.Errorf("Read database entry error", entry, err)
			}
			response.Data = map[string]interface{}{entry.Key(): response.Data}
		}
	}

	handler.responder.WriteResponse(ctx, writer, response)
}
