package handler

import (
	"encoding/json"
	"github.com/exgphe/kin-openapi/openapi3filter"
	"github.com/exgphe/kin-openapi/routers"
	"github.com/exgphe/kin-openapi/routers/legacy"
	"github.com/exgphe/kin-openapi/routers/legacy/pathpattern"
	"github.com/muonsoft/openapi-mock/database"
	"github.com/muonsoft/openapi-mock/internal/openapi/generator"
	"github.com/muonsoft/openapi-mock/internal/openapi/responder"
	"github.com/muonsoft/openapi-mock/pkg/logcontext"
	"github.com/pkg/errors"
	"github.com/spyzhov/ajson"
	"io"
	"io/ioutil"
	"net/http"
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

	route, pathParameters, aErr := (*handler.router).FindRoute(request)
	templateFileName := "requests.json"
	_, err := os.Stat(templateFileName)
	initialized := !os.IsNotExist(err)
	if !initialized {
		templateDb := database.Database{Content: ajson.ObjectNode("", make(map[string]*ajson.Node))}

		node := handler.router.Node()
		var targetSuffix pathpattern.Suffix
		for _, suffix := range node.Suffixes {
			if suffix.Pattern == "GET " {
				targetSuffix = suffix
				break
			}
		}
		dataRoots := targetSuffix.Node.Suffixes[0].Node.Suffixes[0].Node.Suffixes[0].Node.Suffixes
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
		http.NotFound(writer, request)
		return
	}
	if route == nil || aErr != nil {
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
		http.Error(writer, "400 Bad Request", http.StatusBadRequest)
		logger.Infof("Route '%s %s' does not pass validation: %v", request.Method, request.URL, err.Error())

		return
	}
	var operation = route.Operation

	response, err := handler.responseGenerator.GenerateResponse(request, route)
	if err != nil {
		handler.responder.WriteError(ctx, writer, err)
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

	keyPath := database.RestconfPathToKeyPath(request.URL.Path, operation)

	if request.Body != http.NoBody && request.Body != nil && !strings.Contains(request.URL.Path, "restconf/operations/") {
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
				//requestData, err := handler.responseGenerator.GenerateRequestData(request, route)
				//exampleBodyData, _ := json.Marshal(requestData)
				//exampleBodyNode, _ := ajson.Unmarshal(exampleBodyData)
				switch request.Method {
				case "POST", "PUT": // TODO not correct
					bodyObject, err := body.GetObject()
					if err != nil {
						logger.Errorf("Body is not an object", err)
					} else {
						var underlyingNode *ajson.Node
						var topKey string
						for key := range bodyObject {
							underlyingNode = bodyObject[key]
							topKey = key
						}
						if underlyingNode == nil {
							logger.Errorf("body is empty", err)
						} else {
							isArray := false
							if underlyingNode.IsObject() && len(underlyingNode.Keys()) == 1 {
								key := underlyingNode.Keys()[0]
								possiblyArray, err := underlyingNode.GetKey(key)
								if err == nil && possiblyArray.IsArray() {
									arr, _ := possiblyArray.GetArray()
									arrKeyPath := keyPath + "[\"" + key + "\"]"
									if request.Method == "POST" {
										err := db.SetArrayNode(arrKeyPath, arr)
										if err != nil {
											logger.Errorf("Cannot set array node", err)
										}
									} else { // "PUT"
										listKey := operation.RequestBody.Value.Content["application/yang-data+json"].Schema.Value.Properties[topKey].Value.Properties[key].Value.ExtensionProps.Extensions["x-key"].(json.RawMessage)
										var listKeyString string
										err := json.Unmarshal(listKey, &listKeyString)
										if err != nil {
											logger.Errorf("Unmarshal x-key error", err)
										}
										for _, node := range arr {
											err := db.AppendNode(arrKeyPath, node, listKeyString) // verify id
											if err != nil {
												logger.Errorf("Cannot append node", err)
											}
										}
									}
									isArray = true
								}
							}
							if !isArray {
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
							//nodes, err := exampleBodyNode.JSONPath(keyPath[1:])
							//if err != nil {
							//	logger.Errorf("Example node json path error", nodes, keyPath, err)
							//} else if len(nodes) != 1 {
							//	logger.Errorf("example node json path not unique", nodes, keyPath, err)
							//} else {
							//	node := nodes[0]
							//	if node.IsArray() {
							//		if underlyingNode.IsArray() {
							//			arr, _ := underlyingNode.GetArray()
							//			err = db.SetArrayNode(keyPath, arr)
							//		} else {
							//			err = db.AppendNode(keyPath, underlyingNode)
							//		}
							//		if err != nil {
							//			logger.Errorf("Cannot Append Node", err)
							//		}
							//	} else {
							//		if underlyingNode.IsArray() {
							//			arr, _ := underlyingNode.GetArray()
							//			err = db.SetArrayNode(keyPath, arr)
							//		} else {
							//			obj, _ := underlyingNode.GetObject()
							//			err = db.SetObjectNode(keyPath, obj)
							//		}
							//		if err != nil {
							//			logger.Errorf("Cannot Set Node", err)
							//		}
							//	}
							//}
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
	if request.Method == "DELETE" {
		err := db.Set(keyPath, nil)
		if err != nil {
			logger.Errorf("Cannot Delete Node", keyPath, err)
		}
	}
	// Try to read from database
	if request.Method == "GET" && !strings.Contains(request.URL.Path, "restconf/operations/") {
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
			response.Data = map[string]interface{}{namespacedKey: response.Data}
		} else if err.Error() == "Key Path Empty Error" {
			http.NotFound(writer, request)
			logger.Debugf("Route '%s %s' was not found", request.Method, request.URL)
			return
		}
	}

	handler.responder.WriteResponse(ctx, writer, response)
}
