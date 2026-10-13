package utils

import (
	"encoding/json"
)

func MustJsonMarshal(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func MustJsonMarshalByte(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func MustJsonMarshalIndent(v any, prefix, indent string) string {
	b, err := json.MarshalIndent(v, prefix, indent)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func MustJsonUnmarshal(data string, v any) {
	err := json.Unmarshal([]byte(data), v)
	if err != nil {
		panic(err)
	}
}
