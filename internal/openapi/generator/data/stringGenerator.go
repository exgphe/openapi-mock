package data

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/exgphe/kin-openapi/openapi3"
	"github.com/lucasjones/reggen"
	"github.com/pkg/errors"
	"math/rand"
	"strconv"
)

type stringGenerator struct {
	random           randomGenerator
	textGenerator    schemaGenerator
	formatGenerators map[string]stringGeneratorFunction
}

func newStringGenerator(random randomGenerator) schemaGenerator {
	generator := &rangedTextGenerator{
		random: random,
	}

	return &stringGenerator{
		random:           random,
		textGenerator:    &textGenerator{generator: generator},
		formatGenerators: defaultFormattedStringGenerators(generator),
	}
}

func (generator *stringGenerator) GenerateDataBySchema(ctx context.Context, schema *openapi3.Schema) (Data, error) {
	var value Data
	var err error
	maxLength := 0
	if schema.MaxLength != nil {
		maxLength = int(*schema.MaxLength)
	}
	if len(schema.Enum) > 0 {
		value = generator.getRandomEnumValue(schema.Enum)
	} else if schema.Pattern != "" {
		_, ok := schema.Extensions["x-range"]
		if ok {
			value, err = generator.generateNumberString(schema)
		} else {
			value, err = generator.generateValueByPattern(schema.Pattern, maxLength)
		}
	} else if formatGenerator, isSupported := generator.formatGenerators[schema.Format]; isSupported {
		value = formatGenerator(int(schema.MinLength), maxLength)
	} else {
		value, err = generator.textGenerator.GenerateDataBySchema(ctx, schema)
	}

	return value, err
}

func (generator *stringGenerator) getRandomEnumValue(enum []interface{}) string {
	return fmt.Sprint(enum[generator.random.Intn(len(enum))])
}

func (generator *stringGenerator) generateValueByPattern(pattern string, maxLength int) (string, error) {
	g, err := reggen.NewGenerator(pattern)
	if err != nil {
		return "", errors.WithStack(&ErrGenerationFailed{
			GeneratorID: "stringGenerator",
			Message:     fmt.Sprintf("cannot generate string value by pattern '%s'", pattern),
			Previous:    err,
		})
	}
	if maxLength > 39 {
		maxLength = 39
	}
	value := g.Generate(maxLength)
	return fmt.Sprintf("%s", value), nil
}

func (generator *stringGenerator) generateNumberString(schema *openapi3.Schema) (string, error) {
	var xType string
	err := json.Unmarshal(schema.Extensions["x-type"].(json.RawMessage), &xType)
	if err != nil {
		return "", err
	}
	var ranges []map[string]interface{}
	err = json.Unmarshal(schema.Extensions["x-range"].(json.RawMessage), &ranges)
	if err != nil {
		return "", err
	}
	rang := ranges[rand.Intn(len(ranges))]
	min, max := rang["min"].(float64), rang["max"].(float64)
	if xType == "decimal64" {
		var fractionDigits int
		err = json.Unmarshal(schema.Extensions["x-fraction-digits"].(json.RawMessage), &fractionDigits)
		if err != nil {
			return "", err
		}
		return strconv.FormatFloat(min+rand.Float64()*(max-min), 'f', fractionDigits, 64), nil
	} else {
		if xType == "uint64" {
			return strconv.FormatUint(uint64(min+rand.Float64()*max), 10), nil
		} else {
			// "int64"
			return strconv.FormatInt(int64(min+rand.Float64()*max), 10), nil
		}
	}
}
