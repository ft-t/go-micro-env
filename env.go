package go_micro_env

import (
	"encoding/json"
	"github.com/imdario/mergo"
	"go-micro.dev/v4/config/source"
	"os"
	"reflect"
	"strings"
	"time"
)

type strippedPrefixKey struct{}
type prefixKey struct{}

var (
	DefaultPrefixes []string
)

type env struct {
	prefixes             []string
	strippedPrefixes     []string
	opts                 source.Options
	targetConfigInstance interface{}
}

func skipPointers(expectedToBeStruct reflect.Type) reflect.Type {
	if expectedToBeStruct == nil {
		return nil
	}

	if expectedToBeStruct.Kind() == reflect.Ptr {
		n := skipPointers(expectedToBeStruct.Elem())
		return n
	}

	return expectedToBeStruct
}

func (e *env) extractTargetKind(pathKeys []string) (finalKind reflect.Kind) {
	finalKind = reflect.Invalid

	if len(pathKeys) == 0 {
		return finalKind
	}

	if e.targetConfigInstance != nil {
		mainType := skipPointers(reflect.TypeOf(e.targetConfigInstance))

		field, found := mainType.FieldByName(pathKeys[0])

		if found {
			for i := 1; i <= len(pathKeys); i++ {
				if i == len(pathKeys) {
					finalKind = field.Type.Kind()
					break
				} else {
					recordType := skipPointers(field.Type)

					if recordType.Kind() == reflect.Map { // we can not go deep into map for now
						valueType := recordType.Elem()

						return valueType.Kind()
					}

					field, found = skipPointers(field.Type).FieldByName(pathKeys[i])

					if !found {
						break
					}
				}
			}
		}
	}

	return finalKind
}

var (
	supportedKinds = []reflect.Kind{
		reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Float32,
		reflect.Float64,
		reflect.String,
	}
)

func contains(s []reflect.Kind, e reflect.Kind) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func (e *env) Read() (*source.ChangeSet, error) {
	var changes map[string]interface{}

	for _, env := range os.Environ() {

		if len(e.prefixes) > 0 || len(e.strippedPrefixes) > 0 {
			notFound := true

			if _, ok := matchPrefix(e.prefixes, env); ok {
				notFound = false
			}

			if match, ok := matchPrefix(e.strippedPrefixes, env); ok {
				env = strings.TrimPrefix(env, match)
				notFound = false
			}

			if notFound {
				continue
			}
		}

		pair := strings.SplitN(env, "=", 2)
		value := pair[1]
		keys := strings.Split(pair[0], "_")
		reverse(keys)

		tmpValue := strings.TrimSpace(value)

		tmp := make(map[string]interface{})
		for i, k := range keys {
			if i == 0 {
				if strings.HasPrefix(tmpValue, "{") && strings.HasSuffix(tmpValue, "}") {
					rec := map[string]interface{}{}

					if err := json.Unmarshal([]byte(tmpValue), &rec); err != nil {
						panic(err)
					} else {
						tmp[k] = rec
					}

				} else if strings.HasPrefix(tmpValue, "[") && strings.HasSuffix(tmpValue, "]") {
					var records []interface{}

					if err := json.Unmarshal([]byte(tmpValue), &records); err != nil {
						panic(err)
					} else {
						tmp[k] = records
					}
				} else {
					tmp[k] = value
				}
				continue
			}

			tmp = map[string]interface{}{k: tmp}
		}

		if err := mergo.Map(&changes, tmp); err != nil {
			return nil, err
		}
	}

	b, err := e.opts.Encoder.Encode(changes)
	if err != nil {
		return nil, err
	}

	cs := &source.ChangeSet{
		Format:    e.opts.Encoder.String(),
		Data:      b,
		Timestamp: time.Now(),
		Source:    e.String(),
	}
	cs.Checksum = cs.Sum()

	return cs, nil
}

func matchPrefix(pre []string, s string) (string, bool) {
	for _, p := range pre {
		if strings.HasPrefix(s, p) {
			return p, true
		}
	}

	return "", false
}

func reverse(ss []string) {
	for i := len(ss)/2 - 1; i >= 0; i-- {
		opp := len(ss) - 1 - i
		ss[i], ss[opp] = ss[opp], ss[i]
	}
}

func (e *env) Watch() (source.Watcher, error) {
	return source.NewNoopWatcher()
}

func (e *env) Write(cs *source.ChangeSet) error {
	return nil
}

func (e *env) String() string {
	return "env"
}

// NewSource returns a config source for parsing ENV variables.
// Underscores are delimiters for nesting, and all keys are lowercased.
//
// Example:
//      "DATABASE_SERVER_HOST=localhost" will convert to
//
//      {
//          "database": {
//              "server": {
//                  "host": "localhost"
//              }
//          }
//      }
func NewSource(targetConfigInstance interface{}, opts ...source.Option) source.Source {
	options := source.NewOptions(opts...)

	var sp []string
	var pre []string
	if p, ok := options.Context.Value(strippedPrefixKey{}).([]string); ok {
		sp = p
	}

	if p, ok := options.Context.Value(prefixKey{}).([]string); ok {
		pre = p
	}

	if len(sp) > 0 || len(pre) > 0 {
		pre = append(pre, DefaultPrefixes...)
	}
	return &env{prefixes: pre, strippedPrefixes: sp, opts: options, targetConfigInstance: targetConfigInstance}
}
