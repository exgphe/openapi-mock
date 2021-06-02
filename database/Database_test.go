package database

import (
	"fmt"
	"github.com/spyzhov/ajson"
	"testing"
)

func TestAJson(t *testing.T) {
	json := []byte(`{"store": {"book": [
{"category": "reference", "author": "Nigel Rees", "title": "Sayings of the Century", "price": 8.95}, 
{"category": "fiction", "author": "Evelyn Waugh", "title": "Sword of Honour", "price": 12.99}, 
{"category": "fiction", "author": "Herman Melville", "title": "Moby Dick", "isbn": "0-553-21311-3", "price": 8.99}, 
{"category": "fiction", "author": "J. R. R. Tolkien", "title": "The Lord of the Rings", "isbn": "0-395-19395-8", "price": 22.99}], 
"bicycle": {"color": "red", "price": 19.95}, "tools": null}}`)
	root := ajson.Must(ajson.Unmarshal(json))
	result := ajson.Must(ajson.Eval(root, "avg($..price)"))
	err := root.AppendObject("price(avg)", result)
	if err != nil {
		panic(err)
	}
	marshalled, err := ajson.Marshal(root)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s", marshalled)
}
