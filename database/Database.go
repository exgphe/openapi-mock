package database

import (
	"encoding/json"
	"github.com/exgphe/kin-openapi/openapi3"
	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
	"github.com/spyzhov/ajson"
	"io/fs"
	"io/ioutil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Key = string

type Value = *ajson.Node

type KeyPath = string

type Dictionary = *ajson.Node

type Database struct {
	Content Dictionary
	m       sync.Mutex
}

const lastModifiedKey = "@@last-modified"
const eTagKey = "@@etag"

type KeyPathNotFoundError struct {
}

type KeyPathEmptyError struct {
}
type KeyPathNotUniqueError struct {
}
type DataExistsError struct {
}

func (e *KeyPathEmptyError) Error() string { return "Key Path Empty Error" }

func (e *KeyPathNotUniqueError) Error() string { return "Key Path Not Unique" }

func (e *KeyPathNotFoundError) Error() string { return "Key Path Not Found" }

func (e *DataExistsError) Error() string { return "409" }

func NewDatabase() (db *Database) {
	db = &Database{
		Content: ajson.ObjectNode("", make(map[string]*ajson.Node)),
	}
	_ = db.Modified()
	return
}

func Load(filename string) (db *Database, err error) {
	_, err = os.Stat(filename)
	if !os.IsNotExist(err) {
		// file exists
		var fileContent []byte
		fileContent, err = ioutil.ReadFile(filename)
		if err != nil {
			return
		}
		db = &Database{
			Content: nil,
		}
		db.Content, err = ajson.Unmarshal(fileContent)
	}
	return
}

func RestconfPathToKeyPath(restconfPath string, operation *openapi3.Operation) (keyPath string, err error) {
	restconfPath = strings.Replace(restconfPath, "/restconf/data/", "", 1)
	layers := strings.Split(restconfPath, "/")
	keyPath = "$"
	pathParameterKeys := []string{}
	for _, parameter := range operation.Parameters {
		if parameter.Value.In == "path" {
			originalName, ok := parameter.Value.Extensions["x-original-name"]
			if ok {
				var name string
				err := json.Unmarshal(originalName.(json.RawMessage), &name)
				if err != nil {
					return "", err
				}
				pathParameterKeys = append(pathParameterKeys, name)
			} else {
				pathParameterKeys = append(pathParameterKeys, parameter.Value.Name)
			}
		}
	}
	currentPathParameterKeyIndex := 0
	for _, layer := range layers {
		if strings.Contains(layer, "=") {
			tokens := strings.Split(layer, "=")
			subtokens := strings.Split(tokens[1], ",")
			keyPath = keyPath + "[\"" + tokens[0] + "\"][?("
			for i, subtoken := range subtokens {
				if i > 0 {
					keyPath = keyPath + "&&"
				}
				subtokenUnescaped, err := url.PathUnescape(subtoken)
				if err != nil {
					return "", err
				}
				_, err = strconv.Atoi(subtokenUnescaped)
				if err == nil {
					keyPath = keyPath + "(@[\"" + pathParameterKeys[currentPathParameterKeyIndex] + "\"]==\"" + subtokenUnescaped + "\"||@[\"" + pathParameterKeys[currentPathParameterKeyIndex] + "\"]==" + subtokenUnescaped + ")"
				} else {
					keyPath = keyPath + "@[\"" + pathParameterKeys[currentPathParameterKeyIndex] + "\"]==\"" + subtokenUnescaped + "\""
				}
				currentPathParameterKeyIndex++
			}
			keyPath = keyPath + ")]"
		} else {
			keyPath = keyPath + "[\"" + layer + "\"]"
		}
	}
	return
}

func (db *Database) Save(filename string) (err error) {
	db.m.Lock()
	defer db.m.Unlock()
	fileData, err := ajson.Marshal(db.Content)
	if err != nil {
		return
	}
	err = ioutil.WriteFile(filename, fileData, fs.ModePerm)
	return
}

func (db *Database) Modified() error {
	//db.m.Lock()
	//defer db.m.Unlock()
	err := db.Content.AppendObject(lastModifiedKey, ajson.StringNode(lastModifiedKey, time.Now().Format(time.RFC1123)))
	if err != nil {
		return err
	}
	v4, err := uuid.NewV4()
	if err != nil {
		return err
	}
	err = db.Content.AppendObject(eTagKey, ajson.StringNode(eTagKey, "\""+v4.String()+"\""))
	if err != nil {
		return err
	}
	return nil
}

func (db *Database) GetLastModified() (lastModified string, err error) {
	lastModifiedNode, err := db.Content.GetKey(lastModifiedKey)
	if err != nil {
		return
	}
	return lastModifiedNode.GetString()
}

func (db *Database) GetETag() (eTag string, err error) {
	eTagNode, err := db.Content.GetKey(eTagKey)
	if err != nil {
		return
	}
	return eTagNode.GetString()
}

func (db *Database) Get(keyPath KeyPath) (value Value, parentIsArray bool, err error) {
	nodes, err := db.Content.JSONPath(keyPath)
	if err != nil {
		return
	}
	if len(nodes) == 0 {
		err = &KeyPathEmptyError{}
		return
	}
	if len(nodes) > 1 {
		err = &KeyPathNotUniqueError{}
		return
	}
	value = nodes[0]
	parentIsArray = value.Parent().IsArray()
	return
}

// EnsureKeyPath Only supports $.aa.bb.cc or $.aa.bb[?(@.id=='123')].cc
func (db *Database) EnsureKeyPath(keyPath KeyPath) (err error) {
	db.m.Lock()
	defer db.m.Unlock()
	paths, err := ajson.ParseJSONPath(keyPath)
	if err != nil {
		return
	}
	paths = paths[1:]
	currentNode := db.Content
	for i, path := range paths {
		if strings.HasPrefix(path, "\"") {
			path = path[1:]
		}
		if strings.HasSuffix(path, "\"") {
			path = path[:len(path)-1]
		}
		if strings.Contains(path, "?") && strings.Contains(path, "@") {
			nodes, err := currentNode.JSONPath("$[" + path + "]")
			if err != nil {
				return err
			}
			if len(nodes) > 1 {
				return &KeyPathNotUniqueError{}
			}
			if len(nodes) == 0 {
				if !currentNode.IsArray() {
					err := currentNode.SetArray([]*ajson.Node{})
					if err != nil {
						return err
					}
				}
				return nil // TODO
				//currentNode.AppendArray()
			}
			currentNode = nodes[0]
			continue
		}
		if !currentNode.HasKey(path) {
			appended := false
			if i < len(paths)-1 {
				nextPath := paths[i+1]
				if strings.Contains(nextPath, "?") && strings.Contains(nextPath, "@") {
					if !currentNode.IsObject() {
						err := currentNode.SetObject(map[string]*ajson.Node{})
						if err != nil {
							return err
						}
					}
					err = currentNode.AppendObject(path, ajson.ArrayNode(path, []*ajson.Node{}))
					if err != nil {
						return
					}
					appended = true
				}
			}
			if !appended {
				if !currentNode.IsObject() {
					err := currentNode.SetObject(map[string]*ajson.Node{})
					if err != nil {
						return err
					}
				}
				err = currentNode.AppendObject(path, ajson.ObjectNode(path, make(map[string]*ajson.Node)))
				if err != nil {
					return
				}
			}
		}
		currentNode, err = currentNode.GetKey(path)
		if err != nil {
			return
		}
	}
	return
}

func (db *Database) Delete(keyPath KeyPath) (err error) {
	db.m.Lock()
	defer db.m.Unlock()
	//err = db.EnsureKeyPath(keyPath)
	//if err != nil {
	//	return
	//}
	nodes, err := db.Content.JSONPath(keyPath)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return &KeyPathNotFoundError{}
	}
	if len(nodes) != 1 {
		return &KeyPathNotUniqueError{}
	}
	node := nodes[0]
	err = node.Delete()
	if err != nil {
		return err
	}
	err = db.Modified()
	if err != nil {
		return err
	}
	return
}

//func (db *Database) SetObjectNode(keyPath KeyPath, value map[string]*ajson.Node) (err error) {
//	err = db.EnsureKeyPath(keyPath)
//	if err != nil {
//		return
//	}
//	nodes, err := db.Content.JSONPath(keyPath)
//	if err != nil {
//		return err
//	}
//	if len(nodes) == 0 {
//		paths, _ := ajson.ParseJSONPath(keyPath)
//		lastPath := paths[len(paths)-1]
//		if strings.Contains(lastPath, "?(@") {
//			parentKeyPath := "$"
//			for _, s := range paths[1 : len(paths)-1] {
//				parentKeyPath += "[" + s + "]"
//			}
//			nodes, err := db.Content.JSONPath(parentKeyPath)
//			if err != nil {
//				return err
//			}
//			if len(nodes) != 1 {
//				return errors.New("KeyPath Not Unique 2")
//			}
//			node := nodes[0]
//			err = node.AppendArray(ajson.ObjectNode(",", value))
//			return err
//		} else {
//			return &KeyPathEmptyError{}
//		}
//	}
//	if len(nodes) != 1 {
//		return errors.New("KeyPath Not Unique")
//	}
//	node := nodes[0]
//	err = node.SetObject(value)
//	return
//}

//func (db *Database) SetArrayNode(keyPath KeyPath, value []*ajson.Node) (err error) {
//	err = db.EnsureKeyPath(keyPath)
//	if err != nil {
//		return
//	}
//	nodes, err := db.Content.JSONPath(keyPath)
//	if err != nil {
//		return err
//	}
//	if len(nodes) != 1 {
//		return errors.New("KeyPath Not Unique")
//	}
//	node := nodes[0]
//	err = node.SetArray(value)
//	return
//}

//func (db *Database) AppendNode(keyPath KeyPath, value *ajson.Node, listKey string) (err error) {
//	err = db.EnsureKeyPath(keyPath)
//	if err != nil {
//		return
//	}
//	nodes, err := db.Content.JSONPath(keyPath)
//	if err != nil {
//		return err
//	}
//	if len(nodes) != 1 {
//		return errors.New("KeyPath Not Unique")
//	}
//	node := nodes[0]
//	if !node.IsArray() {
//		err = node.SetArray(make([]*ajson.Node, 0))
//		if err != nil {
//			return
//		}
//	}
//	if len(listKey) == 0 {
//		err = node.AppendArray(value)
//	} else {
//		children, err := node.GetArray()
//		if err != nil {
//			return err
//		}
//		listKeys := strings.Split(listKey, ",")
//		for _, child := range children {
//			match := true
//			for _, key := range listKeys {
//				aContent, err := child.GetKey(key)
//				if err != nil {
//					return err
//				}
//				bContent, err := value.GetKey(key)
//				if err != nil {
//					return err
//				}
//				aValue, err := aContent.Value()
//				if err != nil {
//					return err
//				}
//				bValue, err := bContent.Value()
//				if err != nil {
//					return err
//				}
//				if aValue != bValue {
//					match = false
//					break
//				}
//			}
//			if match {
//				nodeObject, err := value.GetObject()
//				if err != nil {
//					return err
//				}
//				err = child.SetObject(nodeObject)
//				if err != nil {
//					return err
//				}
//				return err
//			}
//		}
//		err = node.AppendArray(value)
//	}
//	return
//}

func (db *Database) Put(keyPath string, node *ajson.Node) (created bool, err error) {
	db.m.Lock()
	defer db.m.Unlock()
	nodes, err := db.Content.JSONPath(keyPath)
	if err != nil {
		return
	}
	if len(nodes) == 0 {
		created = true
	}
	err = db.EnsureKeyPath(keyPath)
	nodes, err = db.Content.JSONPath(keyPath)
	if err != nil {
		return
	}
	if len(nodes) == 0 {
		// Is Array
		paths, _ := ajson.ParseJSONPath(keyPath)
		lastPath := paths[len(paths)-1]
		if strings.Contains(lastPath, "?") && strings.Contains(lastPath, "@") {
			parentKeyPath := "$"
			for _, s := range paths[1 : len(paths)-1] {
				parentKeyPath += "[" + s + "]"
			}
			nodes, err := db.Content.JSONPath(parentKeyPath)
			if err != nil {
				return false, err
			}
			if len(nodes) != 1 {
				return false, &KeyPathNotUniqueError{}
			}
			nodeElements, err := node.GetArray()
			if err != nil {
				return false, err
			}
			err = nodes[0].AppendArray(nodeElements[0])
			return true, err
		} else {
			return false, &KeyPathEmptyError{}
		}
	}
	if len(nodes) != 1 {
		return false, &KeyPathNotUniqueError{}
	}
	targetNode := nodes[0]
	nodeType := node.Type()
	switch nodeType {
	case ajson.Null:
		err = targetNode.SetNull()
		if err != nil {
			return false, err
		}
	case ajson.Array:
		value, err := node.GetArray()
		if err != nil {
			return false, err
		}
		valueObject, err := value[0].GetObject()
		if err != nil {
			return false, err
		}
		err = targetNode.SetObject(valueObject)
		if err != nil {
			return false, err
		}
	case ajson.Bool:
		value, err := node.GetBool()
		if err != nil {
			return false, err
		}
		err = targetNode.SetBool(value)
		if err != nil {
			return false, err
		}
	case ajson.Numeric:
		value, err := node.GetNumeric()
		if err != nil {
			return false, err
		}
		err = targetNode.SetNumeric(value)
		if err != nil {
			return false, err
		}
	case ajson.String:
		value, err := node.GetString()
		if err != nil {
			return false, err
		}
		err = targetNode.SetString(value)
		if err != nil {
			return false, err
		}
	case ajson.Object:
		value, err := node.GetObject()
		if err != nil {
			return false, err
		}
		err = targetNode.SetObject(value)
		if err != nil {
			return false, err
		}
	default:
		return false, errors.New("Should not happen")
	}
	err = db.Modified()
	if err != nil {
		return false, err
	}
	return
}

func (db *Database) Post(keyPath string, node *ajson.Node, key string, listKeys []string) (appendKey string, err error) {
	db.m.Lock()
	defer db.m.Unlock()
	err = db.EnsureKeyPath(keyPath)
	if err != nil {
		return
	}
	parentNodes, err := db.Content.JSONPath(keyPath)
	if err != nil {
		return
	}
	if len(parentNodes) != 1 {
		return "", &KeyPathNotUniqueError{}
	}
	parentNode := parentNodes[0]
	currentKeyPath := keyPath + "[\"" + key + "\"]"
	currentNodes, err := db.Content.JSONPath(currentKeyPath)
	if err != nil {
		return "", err
	}
	if node.IsArray() {
		nodeArray, err := node.GetArray()
		if err != nil {
			return "", err
		}
		if len(nodeArray) == 0 {
			return "", errors.New("No Item Found in the List")
		}
		if len(nodeArray) > 1 {
			return "", errors.New("Cannot Create Multiple List Items at One Time")
		}
		element := nodeArray[0]
		err = db.EnsureKeyPath(currentKeyPath)
		if err != nil {
			return "", err
		}
		currentArrayElementKeyPath := currentKeyPath + "[?("
		appendKey = key + "="
		for i, listKey := range listKeys {
			value, err := element.GetKey(listKey)
			if err != nil {
				return "", err
			}
			var valueString string
			switch value.Type() {
			case ajson.String:
				valueString, _ = value.GetString()
				currentArrayElementKeyPath += "@[\"" + listKey + "\"]==\"" + valueString + "\""
			case ajson.Bool:
				realValue, _ := value.GetBool()
				if realValue {
					valueString = "true"
				} else {
					valueString = "false"
				}
				currentArrayElementKeyPath += "@[\"" + listKey + "\"]==" + valueString
			case ajson.Numeric:
				realValue, _ := value.GetNumeric()
				valueString = strconv.Itoa(int(realValue))
				currentArrayElementKeyPath += "@[\"" + listKey + "\"]==" + valueString
			default:
				return "", errors.New("Complex Key Type Currently Not Supported")
			}
			if i != 0 {
				appendKey += ","
				currentArrayElementKeyPath += "&&"
			}
			appendKey += url.PathEscape(valueString)
		}
		currentArrayElementKeyPath += ")]"
		currentArrayElementNodes, err := db.Content.JSONPath(currentArrayElementKeyPath)
		if err != nil {
			return "", err
		}
		if len(currentArrayElementNodes) > 0 {
			return "", &DataExistsError{}
		}
		if len(currentNodes) == 0 {
			return "", &KeyPathEmptyError{}
		}
		currentNode := currentNodes[0]
		if !currentNode.IsArray() {
			err := currentNode.SetArray([]*ajson.Node{})
			if err != nil {
				return "", err
			}
		}
		err = currentNode.AppendArray(element)
		if err != nil {
			return "", err
		}
	} else {
		if len(currentNodes) > 0 {
			currentNode := currentNodes[0]
			if !currentNode.IsNull() {
				return "", &DataExistsError{}
			}
		}
		err = parentNode.AppendObject(key, node)
		appendKey = key
		if err != nil {
			return "", err
		}
	}
	err = db.Modified()
	if err != nil {
		return
	}
	return
}

func (db *Database) Patch(keyPath string, patchNode *ajson.Node) (err error) {
	db.m.Lock()
	defer db.m.Unlock()
	parentNodes, err := db.Content.JSONPath(keyPath)
	if err != nil {
		return
	}
	if len(parentNodes) == 0 {
		return &KeyPathNotFoundError{}
	}
	if len(parentNodes) != 1 {
		return &KeyPathNotUniqueError{}
	}
	parentNode := parentNodes[0]
	var object map[string]*ajson.Node
	if patchNode.IsArray() {
		array, _ := patchNode.GetArray()
		object, err = array[0].GetObject()
	} else {
		object, err = patchNode.GetObject()
	}
	if err != nil {
		return err
	}
	for key, node := range object {
		err := parentNode.AppendObject(key, node)
		if err != nil {
			return err
		}
	}
	err = db.Modified()
	if err != nil {
		return err
	}
	return
}
