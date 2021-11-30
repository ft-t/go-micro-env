package go_micro_env

import (
	"github.com/imdario/mergo"
	"go-micro.dev/v4/config/source"
	"os"
	"reflect"
	"strconv"
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
		originalKeys := strings.Split(pair[0], "_")
		keys := strings.Split(strings.ToLower(pair[0]), "_")
		reverse(keys)

		tmp := make(map[string]interface{})

		for i, k := range keys {
			if i == 0 {
				kindFromConfig := e.extractTargetKind(originalKeys)

				if kindFromConfig != reflect.Invalid {
					switch kindFromConfig {
					case reflect.Bool:
						if v, err := strconv.ParseBool(value); err != nil {
							return nil, err
						} else {
							tmp[k] = v
						}
					case reflect.Int8:
						fallthrough
					case reflect.Int16:
						fallthrough
					case reflect.Int32:
						fallthrough
					case reflect.Int:
						fallthrough
					case reflect.Uint8:
						fallthrough
					case reflect.Uint16:
						fallthrough
					case reflect.Uint32:
						fallthrough
					case reflect.Uint:
						if v, err := strconv.Atoi(value); err != nil {
							return nil, err
						} else {
							tmp[k] = v
						}
					case reflect.Int64:
						fallthrough
					case reflect.Uint64:
						if v, err := strconv.ParseInt(value, 10, 64); err != nil {
							return nil, err
						} else {
							tmp[k] = v
						}
					case reflect.Float64:
						fallthrough
					case reflect.Float32:
						if v, err := strconv.ParseFloat(value, 64); err != nil {
							return nil, err
						} else {
							tmp[k] = v
						}
					case reflect.String:
						tmp[k] = value
					}
				} else {
					if intValue, err := strconv.Atoi(value); err == nil {
						tmp[k] = intValue
					} else if boolValue, err := strconv.ParseBool(value); err == nil {
						tmp[k] = boolValue
					} else {
						tmp[k] = value
					}
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
