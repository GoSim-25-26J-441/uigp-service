package validate

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

var (
	once    sync.Once
	schema  *jsonschema.Schema
	loadErr error
)

func load() {
	c := jsonschema.NewCompiler()
	// allow file refs relative to working dir
	f, err := os.Open("schema/architecture.schema.json")
	if err != nil {
		loadErr = err
		return
	}
	defer f.Close()
	_ = c.AddResource("file://schema/architecture.schema.json", f)
	s, err := c.Compile("file://schema/architecture.schema.json")
	if err != nil {
		loadErr = err
		return
	}
	schema = s
}

// ValidateMap validates a generic map against the schema.
func ValidateMap(m map[string]any) error {
	once.Do(load)
	if loadErr != nil {
		return loadErr
	}
	b, _ := json.Marshal(m)
	var v any
	_ = json.Unmarshal(b, &v)
	return schema.Validate(v)
}
