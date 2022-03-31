package handler

import (
	"encoding/json"
	"github.com/exgphe/kin-openapi/routers/legacy"
	"github.com/muonsoft/openapi-mock/internal/openapi"
	"net/http"
	"strings"
)

type optionsHandler struct {
	router      *legacy.Router
	nextHandler http.Handler
}

func (handler *optionsHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method == "OPTIONS" {
		handler.respond(writer, request)
	} else {
		allowedMethods := handler.getAllowedMethods(request)
		if len(allowedMethods) == 1 {
			handler.notFound(writer, request)
			return
		}
		methodAllowed := false
		for _, method := range allowedMethods {
			if method == request.Method {
				methodAllowed = true
				break
			}
		}
		if methodAllowed {
			handler.nextHandler.ServeHTTP(writer, request)
		} else {
			handler.methodNotAllowed(writer, request)
		}
	}
}

func (handler *optionsHandler) respond(writer http.ResponseWriter, request *http.Request) {
	allowedMethods := handler.getAllowedMethods(request)
	writer.Header().Set("Allow", strings.Join(allowedMethods, ", "))
	acceptPatch := false
	for _, method := range allowedMethods {
		if method == "PATCH" {
			acceptPatch = true
			break
		}
	}
	if acceptPatch {
		writer.Header().Set("Accept-Patch", "application/yang-data+json; charset=UTF-8")
	}
	writer.WriteHeader(http.StatusOK)
}

func (handler *optionsHandler) getAllowedMethods(request *http.Request) []string {
	allowedMethods := []string{"OPTIONS"}
	possibleMethods := []string{"HEAD", "GET", "POST", "PUT", "PATCH", "DELETE"}
	// temporary solution until new routing based on patterns
	originalMethod := request.Method
	for _, method := range possibleMethods {
		request.Method = method
		var err error
		if (strings.HasPrefix(request.URL.Path, "/internal/trigger") || strings.HasPrefix(request.URL.Path, "/restconf/streams/yang-push-json/subscription-id=")) && method == "GET" {
			err = nil
		} else {
			_, _, err = (*handler.router).FindRoute(request)
		}
		if err == nil {
			allowedMethods = append(allowedMethods, method)
		}
	}
	request.Method = originalMethod
	return allowedMethods
}

func (handler *optionsHandler) methodNotAllowed(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/yang-data+json; charset=UTF-8")
	writer.WriteHeader(http.StatusMethodNotAllowed)

	restconfErrors := openapi.NewRestconfErrors(openapi.RestconfError{
		ErrorType:    openapi.ErrorTypeProtocol,
		ErrorTag:     openapi.ErrorTagOperationNotSuported,
		ErrorPath:    request.URL.Path,
		ErrorMessage: "Method Not Allowed",
	})

	marshal, _ := json.Marshal(restconfErrors)

	_, _ = writer.Write(marshal)
}

func (handler *optionsHandler) notFound(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/yang-data+json; charset=UTF-8")
	writer.WriteHeader(http.StatusNotFound)

	restconfErrors := openapi.NewRestconfErrors(openapi.RestconfError{
		ErrorType:    openapi.ErrorTypeProtocol,
		ErrorTag:     openapi.ErrorTagInvalidValue,
		ErrorPath:    request.URL.Path,
		ErrorMessage: "Not Found",
	})

	marshal, _ := json.Marshal(restconfErrors)

	_, _ = writer.Write(marshal)
}
