package handler

import (
	"context"
	"encoding/json"
	"github.com/exgphe/kin-openapi/openapi3"
	"github.com/exgphe/kin-openapi/routers"
	"github.com/exgphe/kin-openapi/routers/legacy"
	"github.com/muonsoft/openapi-mock/database"
	"github.com/muonsoft/openapi-mock/internal/openapi"
	"github.com/muonsoft/openapi-mock/internal/openapi/generator"
	"github.com/muonsoft/openapi-mock/internal/openapi/responder"
	sc "github.com/muonsoft/openapi-mock/internal/openapi/subscriptionCenter"
	"github.com/muonsoft/openapi-mock/openapi-validator"
	"github.com/muonsoft/openapi-mock/pkg/logcontext"
	"github.com/pkg/errors"
	"github.com/spyzhov/ajson"
	"github.com/yudai/gojsondiff"
	"google.golang.org/grpc"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

var subscriptionCenter = sc.NewSubscriptionCenter()

type responseGeneratorHandler struct {
	router            *legacy.Router
	responseGenerator generator.ResponseGenerator
	responder         responder.Responder
	databasePath      string
	grpcPort          uint16
	sseInterval       uint64
}

func NewResponseGeneratorHandler(
	router *legacy.Router,
	responseGenerator generator.ResponseGenerator,
	responder responder.Responder,
	databasePath string,
	grpcPort uint16,
	sseInterval uint64,
) http.Handler {
	generatorHandler := &responseGeneratorHandler{
		router:            router,
		responseGenerator: responseGenerator,
		responder:         responder,
		databasePath:      databasePath,
		grpcPort:          grpcPort,
		sseInterval:       sseInterval,
	}

	return &optionsHandler{
		router:      router,
		nextHandler: generatorHandler,
	}
}

const previousDatabaseFilename = ".temp/database_previous.json"
const afterDatabaseFilename = ".temp/database_after.json"

func (handler *responseGeneratorHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	ctx := request.Context()
	logger := logcontext.LoggerFromContext(ctx)
	if strings.HasPrefix(request.URL.Path, "/internal/trigger") {
		previousDatabaseData, err := ioutil.ReadFile(previousDatabaseFilename)
		if err != nil {
			logger.Errorf("Failed to open file %s", previousDatabaseFilename, err)
			handler.notFound(writer, request)
			return
		}

		// Another JSON string
		afterDatabaseData, err := ioutil.ReadFile(afterDatabaseFilename)
		if err != nil {
			logger.Errorf("Failed to open file %s", afterDatabaseFilename, err)
			handler.notFound(writer, request)
			return
		}
		differ := gojsondiff.New()
		result, err := differ.Compare(previousDatabaseData, afterDatabaseData)
		if err != nil {
			handler.responder.WriteError(ctx, writer, request.URL.Path, err)
			return
		}
		if result.Modified() {
			logger.Infoln("检测到改动")
			//	// TODO detailed diffs
			previousDatabase, err := ajson.Unmarshal(previousDatabaseData)
			if err != nil {
				handler.responder.WriteError(ctx, writer, request.URL.Path, err)
				return
			}
			afterDatabase, err := ajson.Unmarshal(afterDatabaseData)
			if err != nil {
				handler.responder.WriteError(ctx, writer, request.URL.Path, err)
				return
			}
			previousNetworksNode, err := previousDatabase.JSONPath("$['ietf-network:networks'].network")
			if err != nil {
				handler.responder.WriteError(ctx, writer, request.URL.Path, err)
				return
			}
			previousNetworks, _ := previousNetworksNode[0].GetArray()
			for _, previousNetwork := range previousNetworks {
				networkID, _ := previousNetwork.GetKey("network-id")
				networkIDString, _ := networkID.GetString()
				afterNetworkCandidates, _ := afterDatabase.JSONPath("$['ietf-network:networks'].network[?(@['network-id']=='" + networkIDString + "')]")
				if len(afterNetworkCandidates) > 0 {
					afterNetwork := afterNetworkCandidates[0]
					// Compare Nodes
					previousNodesNode, _ := previousNetwork.GetKey("node")
					previousNodes, _ := previousNodesNode.GetArray()
					previousNodesIds := make(map[string]bool)
					for _, previousNode := range previousNodes {
						nodeID, _ := previousNode.GetKey("node-id")
						nodeIDString, _ := nodeID.GetString()
						previousNodesIds[nodeIDString] = true
						afterNodeArray, _ := afterNetwork.JSONPath("@.node[?(@['node-id']=='" + nodeIDString + "')]")
						if len(afterNodeArray) > 0 {
							afterNode := afterNodeArray[0]
							// Compare tp
							previousTpsNode, _ := previousNode.GetKey("ietf-network-topology:termination-point")
							previousTps, _ := previousTpsNode.GetArray()
							previousTpsIds := make(map[string]bool)
							for _, previousTp := range previousTps {
								tpID, _ := previousTp.GetKey("tp-id")
								tpIDString, _ := tpID.GetString()
								previousTpsIds[tpIDString] = true
								afterTpArray, _ := afterNode.JSONPath("@['ietf-network-topology:termination-point'][?(@['tp-id']=='" + tpIDString + "')]")
								if len(afterTpArray) > 0 {
									afterTp := afterTpArray[0]
									eq, _ := previousTp.Eq(afterTp)
									if err != nil {
										logger.Errorf("tp compare error", err)
									} else {
										if !eq {
											value, _ := afterTp.Unpack()
											err := subscriptionCenter.SendAll(openapi.ObjectTypeInfoTP, openapi.OperationUpdate, value, networkIDString, nodeIDString, tpIDString)
											if err != nil {
												logger.Errorf("tp update notification error", err)
												break
											}
										}
									}
								} else {
									err := subscriptionCenter.SendAll(openapi.ObjectTypeInfoTP, openapi.OperationDelete, nil, networkIDString, nodeIDString, tpIDString)
									if err != nil {
										logger.Errorf("tp delete notification error", err)
										break
									}
								}
							}

							afterTpsNode, _ := afterNode.GetKey("ietf-network-topology:termination-point")
							afterTps, _ := afterTpsNode.GetArray()
							for _, afterTp := range afterTps {
								tpID, _ := afterTp.GetKey("tp-id")
								tpIDString, _ := tpID.GetString()
								if !previousTpsIds[tpIDString] {
									value, _ := afterTp.Unpack()
									err := subscriptionCenter.SendAll(openapi.ObjectTypeInfoTP, openapi.OperationCreate, value, networkIDString, nodeIDString, tpIDString)
									if err != nil {
										logger.Errorf("tp added notification error", err)
										break
									}
								}
							}
							// TODO compare ttp
							previousTtps, _ := previousNode.JSONPath("@['ietf-te-topology:te']['tunnel-termination-point'][*]")
							previousTtpsIds := make(map[string]bool)
							for _, previousTtp := range previousTtps {
								ttpID, _ := previousTtp.GetKey("tunnel-tp-id")
								ttpIDString, _ := ttpID.GetString()
								previousTtpsIds[ttpIDString] = true
								afterTtpArray, _ := afterNode.JSONPath("@['ietf-te-topology:te']['tunnel-termination-point'][?(@['tunnel-tp-id']=='" + ttpIDString + "')]")
								if len(afterTtpArray) > 0 {
									afterTtp := afterTtpArray[0]
									eq, _ := previousTtp.Eq(afterTtp)
									if err != nil {
										logger.Errorf("ttp compare error", err)
									} else {
										if !eq {
											value, _ := afterTtp.Unpack()
											err := subscriptionCenter.SendAll(openapi.ObjectTypeInfoTTP, openapi.OperationUpdate, value, networkIDString, nodeIDString, ttpIDString)
											if err != nil {
												logger.Errorf("ttp update notification error", err)
												break
											}
										}
									}
								} else {
									err := subscriptionCenter.SendAll(openapi.ObjectTypeInfoTTP, openapi.OperationDelete, nil, networkIDString, nodeIDString, ttpIDString)
									if err != nil {
										logger.Errorf("ttp delete notification error", err)
										break
									}
								}
							}

							afterTtps, _ := afterNode.JSONPath("@['ietf-te-topology:te']['tunnel-termination-point'][*]")
							for _, afterTtp := range afterTtps {
								ttpID, _ := afterTtp.GetKey("tunnel-tp-id")
								ttpIDString, _ := ttpID.GetString()
								if !previousTtpsIds[ttpIDString] {
									value, _ := afterTtp.Unpack()
									err := subscriptionCenter.SendAll(openapi.ObjectTypeInfoTTP, openapi.OperationCreate, value, networkIDString, nodeIDString, ttpIDString)
									if err != nil {
										logger.Errorf("ttp added notification error", err)
										break
									}
								}
							}

							// Remove tp and ttp
							previousTe, _ := previousNode.GetKey("ietf-te-topology:te")
							previousTtpsNode, _ := previousTe.GetKey("tunnel-termination-point")
							afterTe, _ := afterNode.GetKey("ietf-te-topology:te")
							afterTtpsNode, _ := afterTe.GetKey("tunnel-termination-point")

							// check updated
							_ = previousNode.DeleteKey("ietf-network-topology:termination-point")
							_ = afterNode.DeleteKey("ietf-network-topology:termination-point")
							_ = previousTtpsNode.Delete()
							_ = afterTtpsNode.Delete()

							eq, _ := previousNode.Eq(afterNode)
							if err != nil {
								logger.Errorf("node compare error", err)
							} else {
								if !eq {
									value, _ := afterNode.Unpack()
									err := subscriptionCenter.SendAll(openapi.ObjectTypeInfoNode, openapi.OperationUpdate, value, networkIDString, nodeIDString)
									if err != nil {
										logger.Errorf("node update notification error", err)
										break
									}
								}
							}
						} else {
							err := subscriptionCenter.SendAll(openapi.ObjectTypeInfoNode, openapi.OperationDelete, nil, networkIDString, nodeIDString)
							if err != nil {
								logger.Errorf("node delete notification error", err)
								break
							}
						}
					}
					// check added
					afterNodesNode, _ := afterNetwork.GetKey("node")
					afterNodes, _ := afterNodesNode.GetArray()
					for _, afterNode := range afterNodes {
						nodeID, _ := afterNode.GetKey("node-id")
						nodeIDString, _ := nodeID.GetString()
						if !previousNodesIds[nodeIDString] {
							value, _ := afterNode.Unpack()
							err := subscriptionCenter.SendAll(openapi.ObjectTypeInfoNode, openapi.OperationCreate, value, networkIDString, nodeIDString)
							if err != nil {
								logger.Errorf("node added notification error", err)
								break
							}
						}
					}
					// Compare Links
					previousLinksNode, _ := previousNetwork.GetKey("ietf-network-topology:link")
					previousLinks, _ := previousLinksNode.GetArray()
					previousLinksIds := make(map[string]bool)
					for _, previousLink := range previousLinks {
						linkID, _ := previousLink.GetKey("link-id")
						linkIDString, _ := linkID.GetString()
						previousLinksIds[linkIDString] = true
						afterLinkArray, _ := afterNetwork.JSONPath("@['ietf-network-topology:link'][?(@['link-id']=='" + linkIDString + "')]")
						if len(afterLinkArray) > 0 {
							afterLink := afterLinkArray[0]
							// check updated
							eq, _ := previousLink.Eq(afterLink)
							if err != nil {
								logger.Errorf("link compare error", err)
							} else {
								if !eq {
									value, _ := afterLink.Unpack()
									err := subscriptionCenter.SendAll(openapi.ObjectTypeInfoLink, openapi.OperationUpdate, value, networkIDString, linkIDString)
									if err != nil {
										logger.Errorf("link update notification error", err)
										break
									}
								}
							}
						} else {
							err := subscriptionCenter.SendAll(openapi.ObjectTypeInfoLink, openapi.OperationDelete, nil, networkIDString, linkIDString)
							if err != nil {
								logger.Errorf("link delete notification error", err)
								break
							}
						}
					}
					// check added
					afterLinksNode, _ := afterNetwork.GetKey("ietf-network-topology:link")
					afterLinks, _ := afterLinksNode.GetArray()
					for _, afterLink := range afterLinks {
						linkID, _ := afterLink.GetKey("link-id")
						linkIDString, _ := linkID.GetString()
						if !previousLinksIds[linkIDString] {
							value, _ := afterLink.Unpack()
							err := subscriptionCenter.SendAll(openapi.ObjectTypeInfoLink, openapi.OperationCreate, value, networkIDString, linkIDString)
							if err != nil {
								logger.Errorf("node added notification error", err)
								break
							}
						}
					}
				}
			}
			//err = subscriptionCenter.SendAll(openapi.ObjectTypeInfoNode, openapi.OperationUpdate, nil, networkID)
			//if err != nil {
			//	handler.responder.WriteError(ctx, writer, request.URL.Path, err)
			//	return
			//}
		} else {
			logger.Infoln("未检测到改动")
		}
		handler.responder.WriteResponse(ctx, writer, request.URL.Path, &generator.Response{
			StatusCode:  http.StatusNoContent,
			ContentType: "",
			Data:        nil,
		})
		return
	} else if strings.HasPrefix(request.URL.Path, "/restconf/streams/yang-push-json/subscription-id=") {
		id, err := strconv.Atoi(request.URL.Path[49:])
		if err != nil {
			handler.responder.WriteError(ctx, writer, request.URL.Path, err)
			return
		}
		subscriptions := subscriptionCenter.Get(uint32(id))
		if subscriptions != nil {
			err = subscriptionCenter.Connect(uint32(id), handler.sseInterval, writer, request)
			if err != nil {
				handler.responder.WriteError(ctx, writer, request.URL.Path, err)
				return
			}
		} else {
			handler.notFound(writer, request)
			return
		}
		return
	}

	route, rawPathParameters, aErr := (*handler.router).FindRoute(request)
	pathParameters := map[string]string{}
	for key, value := range rawPathParameters {
		unescape, err := url.PathUnescape(value)
		if err != nil {
			handler.responder.WriteError(ctx, writer, request.URL.Path, err)
			return
		}
		pathParameters[key] = unescape
	}
	if route == nil || aErr != nil {
		handler.notFound(writer, request)

		logger.Debugf("Route '%s %s' was not found", request.Method, request.URL)
		return
	}
	var bodyData []byte
	var err error
	if request.Body != http.NoBody && request.Body != nil {
		bodyData, err = ioutil.ReadAll(request.Body)
		if err != nil {
			handler.badRequest(writer, request, errors.WithMessage(err, "Cannot read body"))
			return
		}
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

	conn, err := grpc.Dial("localhost:"+strconv.Itoa(int(handler.grpcPort)), grpc.WithInsecure()) // TODO don't hard code
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

	filename := handler.databasePath
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

	if strings.Contains(request.URL.Path, "restconf/operations/") {
		switch request.URL.Path {
		case "/restconf/operations/ietf-subscribed-notifications:establish-subscription":
			var requestInput openapi.EstablishSubscriptionInput
			err := json.Unmarshal(bodyData, &requestInput)
			if err == nil {
				if requestInput.Input.Encoding != "" && requestInput.Input.Encoding != "ietf-subscribed-notifications:encode-json" {
					handler.badRequestRestconf(writer, request, openapi.EncodingUnsupportedError())
					return
				}
				id := subscriptionCenter.Subscribe(requestInput.Input.Subscription.Subscription)
				output := openapi.EstablishSubscriptionOutput{
					ID: id,
				}
				response.Data = output.Wrap()
			} else {
				handler.badRequest(writer, request, errors.WithMessage(err, "Cannot extract body"))
				logger.Errorf("Cannot extract body", err)
				return
			}
		case "/restconf/operations/ietf-subscribed-notifications:delete-subscription":
			var requestInput openapi.DeleteSubscriptionInput
			err := json.Unmarshal(bodyData, &requestInput)
			if err == nil {
				id := requestInput.Input.ID
				success := subscriptionCenter.Delete(id)
				if !success {
					handler.badRequestRestconf(writer, request, openapi.NoSuchSubscriptionError())
					return
				}
				response.StatusCode = http.StatusNoContent
				response.Data = nil
			} else {
				handler.badRequest(writer, request, errors.WithMessage(err, "Cannot extract body"))
				logger.Errorf("Cannot extract body", err)
				return
			}
		default:
			break
		}
	} else if request.Method != "DELETE" {
		// Try to read from database
		if request.Method == "GET" || request.Method == "HEAD" {
			entry, parentIsArray, err := db.Get(keyPath)
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
					if !parentIsArray {
						response.Data = map[string]interface{}{namespacedKey: response.Data}
					} else {
						response.Data = map[string]interface{}{namespacedKey: []interface{}{response.Data}}
					}
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
						var listKeys []string
						if ok {
							err := json.Unmarshal(rawXKey.(json.RawMessage), &xKey)
							if err != nil {
								handler.responder.WriteError(ctx, writer, request.URL.Path, err)
								return
							}
							listKeys = strings.Split(xKey, ",")
						}
						switch request.Method {
						case "POST":
							tokens := strings.Split(topKey, ":")
							var subKey string
							if strings.Contains(request.URL.Path, tokens[0]+":") { // TODO seems inaccurate
								subKey = tokens[1]
							} else {
								subKey = topKey
							}
							appendKey, err := db.Post(keyPath, underlyingNode, subKey, listKeys)
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
							// check id does not change
							if handler.checkListKeyLeafValuesChanged(writer, request, underlyingNode, route, pathParameters, listKeys, ctx) {
								return
							}
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
							if handler.checkListKeyLeafValuesChanged(writer, request, underlyingNode, route, pathParameters, listKeys, ctx) {
								return
							}
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
	} else { // DELETE
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

func (handler *responseGeneratorHandler) checkListKeyLeafValuesChanged(writer http.ResponseWriter, request *http.Request, underlyingNode *ajson.Node, route *routers.Route, pathParameters map[string]string, listKeys []string, ctx context.Context) bool {
	if underlyingNode.IsArray() {
		underlyingNodeElements, _ := underlyingNode.GetArray()
		underlyingNodeElement := underlyingNodeElements[0]
		var pathParameterOrder []string
		for _, parameter := range route.Operation.Parameters {
			if parameter.Value.In == "path" {
				pathParameterOrder = append(pathParameterOrder, pathParameters[parameter.Value.Name])
			}
		}
		pathParameterOrderLen := len(pathParameterOrder)
		for i, listKey := range listKeys {
			value, err := underlyingNodeElement.GetKey(listKey)
			if err != nil {
				continue
			}
			if pathParameterOrder[pathParameterOrderLen-i-1] != strings.TrimSuffix(strings.TrimPrefix(value.String(), "\""), "\"") {
				handler.badRequest(writer, request, errors.New("The "+strings.ToUpper(route.Method)+" method MUST NOT be used to change the key leaf values for a data resource instance"))
				return true
			}
		}
	}
	return false
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

func (handler *responseGeneratorHandler) badRequestRestconf(writer http.ResponseWriter, request *http.Request, errs ...openapi.RestconfError) {
	handler.writeError(writer,
		http.StatusBadRequest,
		openapi.NewRestconfErrors(errs...))
}
