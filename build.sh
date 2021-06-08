GOOS=linux GOARCH=amd64 go build -o dist/openapi-mock-linux github.com/muonsoft/openapi-mock/cmd/openapi-mock
GOOS=windows GOARCH=amd64 go build -o dist/openapi-mock-win.exe github.com/muonsoft/openapi-mock/cmd/openapi-mock
GOOS=darwin GOARCH=amd64 go build -o dist/openapi-mock-macos github.com/muonsoft/openapi-mock/cmd/openapi-mock
