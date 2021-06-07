package handler

import (
	"encoding/json"
	"github.com/exgphe/kin-openapi/openapi3"
	"github.com/exgphe/kin-openapi/routers"
	"github.com/exgphe/kin-openapi/routers/legacy"
	"github.com/exgphe/kin-openapi/routers/legacy/pathpattern"
	"github.com/muonsoft/openapi-mock/database"
	"github.com/muonsoft/openapi-mock/internal/openapi"
	"github.com/muonsoft/openapi-mock/internal/openapi/generator"
	"github.com/muonsoft/openapi-mock/internal/openapi/responder"
	"github.com/muonsoft/openapi-mock/openapi-validator"
	"github.com/muonsoft/openapi-mock/pkg/logcontext"
	"github.com/pkg/errors"
	"github.com/spyzhov/ajson"
	"google.golang.org/grpc"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type responseGeneratorHandler struct {
	router            *legacy.Router
	responseGenerator generator.ResponseGenerator
	responder         responder.Responder
}

func NewResponseGeneratorHandler(
	router *legacy.Router,
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

	route, rawPathParameters, aErr := (*handler.router).FindRoute(request)
	templateFileName := "requests.json"
	pathParameters := map[string]string{}
	for key, value := range rawPathParameters {
		unescape, err := url.PathUnescape(value)
		if err != nil {
			handler.responder.WriteError(ctx, writer, request.URL.Path, err)
			return
		}
		pathParameters[key] = unescape
	}
	_, err := os.Stat(templateFileName)
	initialized := !os.IsNotExist(err)
	if !initialized {
		templateDb := database.NewDatabase()

		node := handler.router.Node()
		var targetSuffix pathpattern.Suffix
		for _, suffix := range node.Suffixes {
			if suffix.Pattern == "GET " {
				targetSuffix = suffix
				break
			}
		}
		dataRoots := targetSuffix.Node.Suffixes[0].Node.Suffixes[0].Node.Suffixes[0].Node.Suffixes // GET /restconf/data/*
		for _, root := range dataRoots {
			rootRoute := root.Node.Value.(*routers.Route)
			response, err := handler.responseGenerator.GenerateResponse(request, rootRoute)
			if err != nil {
				logger.Errorf("Create template error", err)
				continue
			}
			responseData, _ := json.Marshal(response.Data)
			responseNode, _ := ajson.Unmarshal(responseData)
			key := responseNode.Keys()[0]
			object, _ := responseNode.GetKey(key)
			err = templateDb.Content.AppendObject(key, object)
			if err != nil {
				logger.Errorf("Whatever error", err)
			}
		}
		err := templateDb.Save(templateFileName)
		if err != nil {
			logger.Errorf("Save Template DB Error", err)
		}
		handler.notFound(writer, request)
		return
	}
	if route == nil || aErr != nil {
		handler.notFound(writer, request)

		logger.Debugf("Route '%s %s' was not found", request.Method, request.URL)
		return
	}
	var bodyData []byte
	if request.Body != http.NoBody && request.Body != nil {
		bodyData, err = ioutil.ReadAll(request.Body)
		defer request.Body.Close()
	}

	//routingValidation := &openapi3filter.RequestValidationInput{
	//	Request:    request,
	//	PathParams: pathParameters,
	//	Route:      route,
	//	Options: &openapi3filter.Options{
	//		ExcludeRequestBody: true,
	//	},
	//}
	//
	//err = openapi3filter.ValidateRequest(ctx, routingValidation)

	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure()) // TODO don't hard code
	if err != nil {
		handler.responder.WriteError(ctx, writer, request.URL.Path, errors.New("Validation Server Down"))
		return
	}
	defer conn.Close()
	validationService := openapi_validator.NewApiClient(conn)

	headers := map[string]string{}

	for key, values := range request.Header {
		headers[strings.ToLower(key)] = strings.Join(values, ", ")
	}
	queries := map[string]string{}
	for key, values := range request.URL.Query() {
		queries[key] = values[0]
	}

	validationResponse, err := validationService.Validate(ctx, &openapi_validator.ValidationRequest{
		Path:               route.Path,
		Method:             request.Method,
		Headers:            headers,
		Params:             pathParameters,
		Query:              queries,
		Body:               bodyData,
		ValidatingResponse: false,
	})
	if err != nil {
		handler.responder.WriteError(ctx, writer, request.URL.Path, errors.WithMessage(err, "Validation Service Error"))
		logger.Errorf("Validation Service Error", err)
		return
	}

	//var requestError *openapi3filter.RequestError
	if !validationResponse.Ok {
		handler.badRequest(writer, request, errors.New(validationResponse.Message))
		logger.Infof("Route '%s %s' does not pass validation: %s", request.Method, request.URL, validationResponse.Message)
		return
	}
	var operation = route.Operation

	response, err := handler.responseGenerator.GenerateResponse(request, route)
	if err != nil {
		handler.responder.WriteError(ctx, writer, request.URL.Path, err)
		return
	}

	filename := "requests.json"
	db, err := database.Load(filename)
	if err != nil {
		logger.Errorf("Json read error", err)
		db = database.NewDatabase()
	}

	defer func() {
		err = db.Save(filename)
		if err != nil {
			logger.Errorf("Save requests error", err)
		}
	}()

	keyPath, err := database.RestconfPathToKeyPath(request.URL.Path, operation)
	if err != nil {
		handler.responder.WriteError(ctx, writer, request.URL.Path, errors.WithMessage(err, "Keypath Convert Error"))
		logger.Errorf("Keypath convert error", err)
		return
	}

	if !strings.Contains(request.URL.Path, "restconf/operations/") && request.Method != "DELETE" {
		// Try to read from database
		if request.Method == "GET" || request.Method == "HEAD" {
			entry, err := db.Get(keyPath)
			if err == nil {
				var namespacedKey string
				for key := range response.Data.(map[string]interface{}) {
					namespacedKey = key
				}
				err = json.Unmarshal(entry.Source(), &response.Data)
				if err != nil {
					logger.Errorf("Read database entry error", entry, err)
				}
				if request.Method == "GET" {
					response.Data = map[string]interface{}{namespacedKey: response.Data}
				}
			} else if err.Error() == "Key Path Empty Error" {
				handler.notFound(writer, request)
				logger.Debugf("Route '%s %s' was not found", request.Method, request.URL)
				return
			}
		} else if request.Method == "POST" || request.Method == "PUT" || request.Method == "PATCH" {
			body, err := ajson.Unmarshal(bodyData)
			if err == nil {
				//requestData, err := handler.responseGenerator.GenerateRequestData(request, route)
				//exampleBodyData, _ := json.Marshal(requestData)
				//exampleBodyNode, _ := ajson.Unmarshal(exampleBodyData)
				bodyObject, err := body.GetObject()
				if err != nil {
					logger.Errorf("Body is not an object", err)
				} else {
					var underlyingNode *ajson.Node
					var topKey string
					hasMultipleKey := false
					for key := range bodyObject {
						if hasMultipleKey {
							handler.badRequest(writer, request, errors.New("Multiple Key in Request Body"))
							logger.Errorf("Multiple Key in Request Body", request.Method, request.URL, bodyObject)
							return
						}
						underlyingNode = bodyObject[key]
						topKey = key
						hasMultipleKey = true
					}
					if underlyingNode == nil {
						logger.Errorf("body is empty", err)
					} else {
						switch request.Method {
						case "POST":
							tokens := strings.Split(topKey, ":")
							var subKey string
							if strings.Contains(request.URL.Path, tokens[0]+":") { // TODO seems inaccurate
								subKey = tokens[1]
							} else {
								subKey = topKey
							}
							value := route.Operation.RequestBody.Value.Content["application/yang-data+json"].Schema.Value
							var topProperty *openapi3.SchemaRef
							topProperty, ok := value.Properties[topKey]
							if !ok {
								for _, ref := range value.OneOf {
									topProperty, ok = ref.Value.Properties[topKey]
									if ok {
										break
									}
								}
							}
							rawXKey, ok := topProperty.Value.Extensions["x-key"]
							var xKey string
							if ok {
								err := json.Unmarshal(rawXKey.(json.RawMessage), &xKey)
								if err != nil {
									handler.responder.WriteError(ctx, writer, request.URL.Path, err)
									return
								}
							}
							appendKey, err := db.Post(keyPath, underlyingNode, subKey, xKey)
							if err != nil {
								switch err.(type) {
								case *database.DataExistsError:
									handler.conflict(writer, request)
								case *database.KeyPathNotFoundError:
									handler.notFound(writer, request)
								default:
									handler.badRequest(writer, request, err)
									logger.Errorf("Post Error", err)
								}
								return
							}
							writer.Header().Add("Location", request.URL.String()+"/"+appendKey)
							response.StatusCode = http.StatusCreated
							break
						case "PUT":
							// TODO check id does not change
							created, err := db.Put(keyPath, underlyingNode)
							if err != nil {
								handler.badRequest(writer, request, err)
								logger.Errorf("Put Error", err)
								return
							}
							if created {
								response.StatusCode = http.StatusCreated
							} else {
								response.StatusCode = http.StatusNoContent
							}
						case "PATCH":
							// TODO check id does not change
							err := db.Patch(keyPath, underlyingNode)
							if err != nil {
								switch err.(type) {
								case *database.KeyPathNotFoundError:
									handler.notFound(writer, request)
								default:
									handler.badRequest(writer, request, err)
									logger.Errorf("Patch Error", err)
								}
								return
							}
						default:
							handler.badRequest(writer, request, errors.New("Should not Happen"))
							logger.Errorf("Should not Happen", request.Method)
							break
						}
					}
				}
			} else {
				handler.badRequest(writer, request, errors.WithMessage(err, "Cannot extract body"))
				logger.Errorf("Cannot extract body", err)
				return
			}
		}
		lastModified, err := db.GetLastModified()
		if err != nil {
			handler.responder.WriteError(ctx, writer, request.URL.Path, err)
			return
		} else {
			writer.Header().Add("Last-Modified", lastModified)
		}
		eTag, err := db.GetETag()
		if err != nil {
			handler.responder.WriteError(ctx, writer, request.URL.Path, err)
			return
		} else {
			writer.Header().Add("ETag", eTag)
		}
	}
	if request.Method == "DELETE" {
		err := db.Delete(keyPath)
		if err != nil {
			switch err.(type) {
			case *database.KeyPathNotFoundError:
				handler.notFound(writer, request)
			default:
				handler.badRequest(writer, request, err)
				logger.Errorf("Cannot Delete Node", keyPath, err)
			}
			return
		}
	}
	handler.responder.WriteResponse(ctx, writer, request.URL.Path, response)
}

func (handler *responseGeneratorHandler) writeError(writer http.ResponseWriter, statusCode int, restconfErrors openapi.RestconfErrors) {
	writer.Header().Set("Content-Type", "application/yang-data+json; charset=UTF-8")
	writer.WriteHeader(statusCode)

	marshal, _ := json.Marshal(restconfErrors)

	_, _ = writer.Write(marshal)
}

func (handler *responseGeneratorHandler) notFound(writer http.ResponseWriter, request *http.Request) {
	handler.writeError(writer,
		http.StatusNotFound,
		openapi.NewRestconfErrors(openapi.RestconfError{
			ErrorType:    openapi.ErrorTypeProtocol,
			ErrorTag:     openapi.ErrorTagInvalidValue,
			ErrorPath:    request.URL.Path,
			ErrorMessage: "Not Found",
		}))
}

func (handler *responseGeneratorHandler) conflict(writer http.ResponseWriter, request *http.Request) {
	handler.writeError(writer,
		http.StatusConflict,
		openapi.NewRestconfErrors(openapi.RestconfError{
			ErrorType:    openapi.ErrorTypeProtocol,
			ErrorTag:     openapi.ErrorTagResourceDenied,
			ErrorPath:    request.URL.Path,
			ErrorMessage: "Data already exists; cannot create new resource",
		}))
}

func (handler *responseGeneratorHandler) badRequest(writer http.ResponseWriter, request *http.Request, err error) {
	handler.writeError(writer,
		http.StatusBadRequest,
		openapi.NewRestconfErrors(openapi.RestconfError{
			ErrorType:    openapi.ErrorTypeProtocol,
			ErrorTag:     openapi.ErrorTagInvalidValue,
			ErrorPath:    request.URL.Path,
			ErrorMessage: err.Error(),
		}))
}
