package config

import (
	"math"
	"time"

	"github.com/muonsoft/openapi-mock/internal/openapi/generator/data"
	"github.com/sirupsen/logrus"
)

type Configuration struct {
	// OpenAPI options
	SpecificationURL string

	// HTTP server options
	CORSEnabled     bool
	Port            uint16
	HTTPSPort       uint16
	ResponseTimeout time.Duration

	// Application options
	DryRun    bool
	Debug     bool
	LogFormat string
	LogLevel  logrus.Level

	// Generation options
	UseExamples     data.UseExamplesEnum
	NullProbability float64
	DefaultMinInt   int64
	DefaultMaxInt   int64
	DefaultMinFloat float64
	DefaultMaxFloat float64
	SuppressErrors  bool
	DatabasePath    string
	GrpcPort        uint16
	SSEInterval     uint64
}

const (
	DefaultGrpcPort        = uint16(50051)
	DefaultPort            = uint16(8080)
	DefaultHTTPSPort       = uint16(8081)
	DefaultResponseTimeout = time.Hour
	DefaultLogLevel        = logrus.InfoLevel
	DefaultNullProbability = 0
	DefaultMaxInt          = int64(math.MaxInt32)
	DefaultMinFloat        = -float64(math.MaxInt32 / 2)
	DefaultMaxFloat        = float64(math.MaxInt32 / 2)
	DefaultSSEInterval     = uint64(15)
)

func (config *Configuration) Dump() map[string]interface{} {
	return map[string]interface{}{
		"SpecificationURL": config.SpecificationURL,
		"CORSEnabled":      config.CORSEnabled,
		"Port":             config.Port,
		"HTTPSPort":        config.HTTPSPort,
		"ResponseTimeout":  config.ResponseTimeout,
		"Debug":            config.Debug,
		"LogFormat":        config.LogFormat,
		"LogLevel":         config.LogLevel,
		"UseExamples":      config.UseExamples,
		"NullProbability":  config.NullProbability,
		"DefaultMinInt":    config.DefaultMinInt,
		"DefaultMaxInt":    config.DefaultMaxInt,
		"DefaultMinFloat":  config.DefaultMinFloat,
		"DefaultMaxFloat":  config.DefaultMaxFloat,
		"SuppressErrors":   config.SuppressErrors,
		"DatabasePath":     config.DatabasePath,
	}
}
