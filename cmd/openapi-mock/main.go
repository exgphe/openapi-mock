package main

import (
	"fmt"
	"github.com/muonsoft/openapi-mock/internal/application"
	"os"
)

var (
	version   string
	buildTime string
)

func main() {
	err := application.Execute(
		application.Version(version),
		application.BuildTime(buildTime),
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
