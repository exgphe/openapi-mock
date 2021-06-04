package handler

import (
	"github.com/exgphe/kin-openapi/routers/legacy"
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
		handler.nextHandler.ServeHTTP(writer, request)
	}
}

func (handler *optionsHandler) respond(writer http.ResponseWriter, request *http.Request) {
	var allowedMethods []string
	possibleMethods := []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

	// temporary solution until new routing based on patterns
	for _, method := range possibleMethods {
		request.Method = method
		_, _, err := (*handler.router).FindRoute(request)
		if err == nil {
			allowedMethods = append(allowedMethods, method)
		}
	}

	writer.Header().Set("Allow", strings.Join(allowedMethods, ","))
	writer.WriteHeader(http.StatusOK)
}
