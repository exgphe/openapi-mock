package di

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/exgphe/kin-openapi/openapi3"
	"github.com/exgphe/kin-openapi/routers"
	"github.com/exgphe/kin-openapi/routers/legacy"
	"github.com/exgphe/kin-openapi/routers/legacy/pathpattern"
	"github.com/gorilla/handlers"
	"github.com/muonsoft/openapi-mock/database"
	"github.com/muonsoft/openapi-mock/internal/application/config"
	responseGenerator "github.com/muonsoft/openapi-mock/internal/openapi/generator"
	"github.com/muonsoft/openapi-mock/internal/openapi/generator/data"
	"github.com/muonsoft/openapi-mock/internal/openapi/handler"
	"github.com/muonsoft/openapi-mock/internal/openapi/loader"
	"github.com/muonsoft/openapi-mock/internal/openapi/responder"
	"github.com/muonsoft/openapi-mock/internal/server"
	"github.com/muonsoft/openapi-mock/internal/server/middleware"
	"github.com/sirupsen/logrus"
	"github.com/spyzhov/ajson"
	"github.com/unrolled/secure"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

type Factory struct {
	configuration *config.Configuration
	logger        logrus.FieldLogger
}

func NewFactory(configuration *config.Configuration) *Factory {
	logger := createLogger(configuration)

	return &Factory{
		configuration: configuration,
		logger:        logger,
	}
}

func init() {
	openapi3.DefineStringFormat("uuid", openapi3.FormatOfStringForUUIDOfRFC4122)
	openapi3.DefineStringFormat("html", "<[^>]+>|&[^;]+;")
}

func (factory *Factory) GetLogger() logrus.FieldLogger {
	return factory.logger
}

func (factory *Factory) CreateSpecificationLoader() loader.SpecificationLoader {
	return loader.New()
}

func (factory *Factory) CreateHTTPHandler(router *legacy.Router) http.Handler {
	generatorOptions := data.Options{
		UseExamples:     factory.configuration.UseExamples,
		NullProbability: factory.configuration.NullProbability,
		DefaultMinInt:   factory.configuration.DefaultMinInt,
		DefaultMaxInt:   factory.configuration.DefaultMaxInt,
		DefaultMinFloat: factory.configuration.DefaultMinFloat,
		DefaultMaxFloat: factory.configuration.DefaultMaxFloat,
		SuppressErrors:  factory.configuration.SuppressErrors,
	}

	dataGeneratorInstance := data.New(generatorOptions)
	responseGeneratorInstance := responseGenerator.New(dataGeneratorInstance)
	apiResponder := responder.New()

	var httpHandler http.Handler
	httpHandler = handler.NewResponseGeneratorHandler(router, responseGeneratorInstance, apiResponder, factory.configuration.DatabasePath, factory.configuration.GrpcPort, factory.configuration.SSEInterval)
	if factory.configuration.CORSEnabled {
		httpHandler = middleware.CORSHandler(httpHandler)
	}

	secureMiddleware := secure.New(secure.Options{
		FrameDeny:             true,
		ContentTypeNosniff:    true,
		BrowserXssFilter:      true,
		ContentSecurityPolicy: "default-src 'self'",
	})

	httpHandler = secureMiddleware.Handler(httpHandler)
	httpHandler = middleware.ContextLoggerHandler(factory.logger, httpHandler)
	httpHandler = middleware.TracingHandler(httpHandler)
	httpHandler = handlers.CombinedLoggingHandler(os.Stdout, httpHandler)
	httpHandler = handlers.RecoveryHandler(
		handlers.RecoveryLogger(factory.logger),
		handlers.PrintRecoveryStack(true),
	)(httpHandler)
	//httpHandler = http.TimeoutHandler(httpHandler, factory.configuration.ResponseTimeout, "")

	return httpHandler
}

func (factory *Factory) CreateHTTPServer() (server.Server, error) {
	logger := factory.GetLogger()
	loggerWriter := logger.(*logrus.Logger).Writer()

	specificationLoader := factory.CreateSpecificationLoader()
	specification, err := specificationLoader.LoadFromURI(factory.configuration.SpecificationURL)
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI specification from '%s': %w", factory.configuration.SpecificationURL, err)
	}

	logger.Infof("OpenAPI specification was successfully loaded from '%s'", factory.configuration.SpecificationURL)

	router, err := legacy.NewRouter(specification)
	if err != nil {
		return nil, fmt.Errorf("failed to build router from OpenAPI specification from '%s': %w", factory.configuration.SpecificationURL, err)
	}
	httpHandler := factory.CreateHTTPHandler(router)

	serverLogger := log.New(loggerWriter, "[HTTP]: ", log.LstdFlags)
	httpServer := server.New(factory.configuration.Port, httpHandler, serverLogger)

	logger.WithFields(factory.configuration.Dump()).Info("OpenAPI mock server was created")

	return httpServer, nil
}

func (factory *Factory) InitializeDatabase() error {
	logger := factory.GetLogger()

	specificationLoader := factory.CreateSpecificationLoader()
	specification, err := specificationLoader.LoadFromURI(factory.configuration.SpecificationURL)
	if err != nil {
		return fmt.Errorf("failed to load OpenAPI specification from '%s': %w", factory.configuration.SpecificationURL, err)
	}

	logger.Infof("OpenAPI specification was successfully loaded from '%s'", factory.configuration.SpecificationURL)

	router, err := legacy.NewRouter(specification)
	if err != nil {
		return fmt.Errorf("failed to build router from OpenAPI specification from '%s': %w", factory.configuration.SpecificationURL, err)
	}
	ctx := context.Background()
	emptyRequest := http.Request{}
	request := emptyRequest.WithContext(ctx)
	templateFileName := factory.configuration.DatabasePath
	templateDb := database.NewDatabase()

	node := router.Node()
	var targetSuffix pathpattern.Suffix
	for _, suffix := range node.Suffixes {
		if suffix.Pattern == "GET " {
			targetSuffix = suffix
			break
		}
	}
	dataRoots := targetSuffix.Node.Suffixes[0].Node.Suffixes[0].Node.Suffixes[0].Node.Suffixes // GET /restconf/data/*
	generatorOptions := data.Options{
		UseExamples:     factory.configuration.UseExamples,
		NullProbability: factory.configuration.NullProbability,
		DefaultMinInt:   factory.configuration.DefaultMinInt,
		DefaultMaxInt:   factory.configuration.DefaultMaxInt,
		DefaultMinFloat: factory.configuration.DefaultMinFloat,
		DefaultMaxFloat: factory.configuration.DefaultMaxFloat,
		SuppressErrors:  factory.configuration.SuppressErrors,
	}

	dataGeneratorInstance := data.New(generatorOptions)
	responseGeneratorInstance := responseGenerator.New(dataGeneratorInstance)
	for _, root := range dataRoots {
		logger.Info("Generating ", root)
		rootRoute := root.Node.Value.(*routers.Route)
		response, err := responseGeneratorInstance.GenerateResponse(request, rootRoute)
		if err != nil {
			logger.Errorf("Create template error", err)
			return err
		}
		responseData, _ := json.Marshal(response.Data)
		responseNode, _ := ajson.Unmarshal(responseData)
		key := responseNode.Keys()[0]
		object, _ := responseNode.GetKey(key)
		err = templateDb.Content.AppendObject(key, object)
		if err != nil {
			logger.Errorf("Whatever error", err)
			return err
		}
	}
	err = templateDb.Save(templateFileName)
	if err != nil {
		logger.Errorf("Save Template DB Error", err)
		return err
	}
	return nil
}

func createLogger(configuration *config.Configuration) *logrus.Logger {
	logger := logrus.New()
	if configuration.DryRun {
		logger.Out = ioutil.Discard
		return logger
	}

	logger.SetLevel(configuration.LogLevel)

	if configuration.LogFormat == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{})
	} else {
		logger.SetFormatter(&logrus.TextFormatter{})
	}

	return logger
}
