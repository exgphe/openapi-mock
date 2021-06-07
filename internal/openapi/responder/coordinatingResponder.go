package responder

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/muonsoft/openapi-mock/internal/openapi"
	"github.com/muonsoft/openapi-mock/internal/openapi/generator"
	"github.com/muonsoft/openapi-mock/internal/openapi/responder/serializer"
	"github.com/muonsoft/openapi-mock/pkg/logcontext"
	"net/http"
	"regexp"
)

type coordinatingResponder struct {
	serializer     serializer.Serializer
	formatGuessers []formatGuess
}

type formatGuess struct {
	format  string
	pattern *regexp.Regexp
}

func (responder *coordinatingResponder) WriteResponse(ctx context.Context, writer http.ResponseWriter, path string, response *generator.Response) {
	format := responder.guessSerializationFormat(response.ContentType)

	data, err := responder.serializer.Serialize(response.Data, format)
	if err != nil {
		responder.WriteError(ctx, writer, path, err)
		return
	}

	if response.ContentType != "" {
		writer.Header().Set("Content-Type", fmt.Sprintf("%s; charset=utf-8", response.ContentType))
	}

	writer.WriteHeader(response.StatusCode)
	writer.Header().Add("Server", "mock-server")
	writer.Header().Add("Cache-Control", "no-cache")
	_, _ = writer.Write(data)
}

func (responder *coordinatingResponder) WriteError(ctx context.Context, writer http.ResponseWriter, path string, err error) {
	logger := logcontext.LoggerFromContext(ctx)
	logger.Errorf("Server Internal Error", err)
	writer.Header().Set("Content-Type", "application/yang-data+json; charset=UTF-8")
	writer.WriteHeader(http.StatusInternalServerError)

	restconfErrors := openapi.NewRestconfErrors(openapi.RestconfError{
		ErrorType:    openapi.ErrorTypeApplication,
		ErrorTag:     openapi.ErrorTagOperationFailed,
		ErrorPath:    path,
		ErrorMessage: err.Error(),
	})

	marshal, _ := json.Marshal(restconfErrors)

	_, _ = writer.Write(marshal)
}

func (responder *coordinatingResponder) guessSerializationFormat(contentType string) string {
	format := "raw"

	for _, guesser := range responder.formatGuessers {
		if guesser.pattern.MatchString(contentType) {
			format = guesser.format
			break
		}
	}

	return format
}
