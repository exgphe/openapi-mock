package database

import (
	"encoding/json"
	"github.com/pkg/errors"
	"github.com/spyzhov/ajson"
	"io/fs"
	"io/ioutil"
	"os"
	"strings"
)

type Key = string

type Value = *ajson.Node

type KeyPath = string

type Dictionary = *ajson.Node

type Database struct {
	Content Dictionary
}

func Load(filename string) (db Database, err error) {
	_, err = os.Stat(filename)
	if !os.IsNotExist(err) {
		// file exists
		var fileContent []byte
		fileContent, err = ioutil.ReadFile(filename)
		if err != nil {
			return
		}
		db.Content, err = ajson.Unmarshal(fileContent)
	}
	return
}

func RestconfPathToKeyPath(restconfPath string) (keyPath string) {
	restconfPath = strings.Replace(restconfPath, "/restconf/data/", "", 1)
	layers := strings.Split(restconfPath, "/")
	keyPath = "$"
	for _, layer := range layers {
		if strings.Contains(layer, "=") {
			tokens := strings.Split(layer, "=")
			if strings.Contains(tokens[1], ",") {
				//subtokens := strings.Split(tokens[1], ",")
				keyPath = keyPath + "[\"" + tokens[0] + "\"][?(@.id==\"" + tokens[1] + "\")]" // TODO bogus!
			} else {
				keyPath = keyPath + "[\"" + tokens[0] + "\"][?(@.id==\"" + tokens[1] + "\")]"
			}
		} else {
			keyPath = keyPath + "[\"" + layer + "\"]"
		}
	}
	return
}

func (db Database) Save(filename string) (err error) {
	fileData, err := ajson.Marshal(db.Content)
	if err != nil {
		return
	}
	err = ioutil.WriteFile(filename, fileData, fs.ModePerm)
	return
}

func (db *Database) Get(keyPath KeyPath) (value Value, err error) {
	nodes, err := db.Content.JSONPath(keyPath)
	if err != nil {
		return
	}
	if len(nodes) != 1 {
		err = errors.New("KeyPath Not Unique")
		return
	}
	value = nodes[0]
	return
}

// EnsureKeyPath Only supports $.aa.bb.cc or $.aa.bb[?(@.id=='123')].cc
func (db *Database) EnsureKeyPath(keyPath KeyPath) (err error) {
	paths, err := ajson.ParseJSONPath(keyPath)
	if err != nil {
		return
	}
	paths = paths[1:]
	currentNode := db.Content
	for _, path := range paths {
		if strings.HasPrefix(path, "\"") {
			path = path[1:]
		}
		if strings.HasSuffix(path, "\"") {
			path = path[:len(path)-1]
		}
		if strings.Contains(path, "?(@") {
			nodes, err := currentNode.JSONPath("$.[" + path + "]")
			if err != nil {
				return err
			}
			if len(nodes) != 1 {
				return errors.New("KeyPath Not Unique")
			}
			currentNode = nodes[0]
			continue
		}
		if !currentNode.HasKey(path) {
			err = currentNode.AppendObject(path, ajson.ObjectNode(path, make(map[string]*ajson.Node)))
			if err != nil {
				return
			}
		}
		currentNode, err = currentNode.GetKey(path)
		if err != nil {
			return
		}
	}
	return
}

func (db *Database) Set(keyPath KeyPath, value interface{}) (err error) {
	err = db.EnsureKeyPath(keyPath)
	if err != nil {
		return
	}
	nodes, err := db.Content.JSONPath(keyPath)
	if err != nil {
		return err
	}
	if len(nodes) != 1 {
		return errors.New("KeyPath Not Unique")
	}
	node := nodes[0]
	marshal, err := json.Marshal(value)
	if err != nil {
		return
	}
	unmarshalled, err := ajson.Unmarshal(marshal)
	if err != nil {
		return
	}
	object, err := unmarshalled.GetObject()
	if err != nil {
		return
	}
	err = node.SetObject(object)
	return
}

func (db *Database) SetObjectNode(keyPath KeyPath, value map[string]*ajson.Node) (err error) {
	err = db.EnsureKeyPath(keyPath)
	if err != nil {
		return
	}
	nodes, err := db.Content.JSONPath(keyPath)
	if err != nil {
		return err
	}
	if len(nodes) != 1 {
		return errors.New("KeyPath Not Unique")
	}
	node := nodes[0]
	err = node.SetObject(value)
	return
}

func (db *Database) SetArrayNode(keyPath KeyPath, value []*ajson.Node) (err error) {
	err = db.EnsureKeyPath(keyPath)
	if err != nil {
		return
	}
	nodes, err := db.Content.JSONPath(keyPath)
	if err != nil {
		return err
	}
	if len(nodes) != 1 {
		return errors.New("KeyPath Not Unique")
	}
	node := nodes[0]
	err = node.SetArray(value)
	return
}

func (db *Database) AppendNode(keyPath KeyPath, value *ajson.Node) (err error) {
	err = db.EnsureKeyPath(keyPath)
	if err != nil {
		return
	}
	nodes, err := db.Content.JSONPath(keyPath)
	if err != nil {
		return err
	}
	if len(nodes) != 1 {
		return errors.New("KeyPath Not Unique")
	}
	node := nodes[0]
	if node.IsArray() {
		err = node.SetArray(make([]*ajson.Node, 0))
		if err != nil {
			return
		}
	}
	err = node.AppendArray(value)
	return
}
