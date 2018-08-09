package main

import (
	"encoding/json"
	"testing"
)

func FuncWithInterface(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func FuncWithTyped(v map[string]interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func value() map[string]interface{} {
	return map[string]interface{}{
		"test": "hi",
	}
}

func BenchmarkFuncWithInterface(b *testing.B) {
	for i := 0; i < b.N; i++ {
		FuncWithInterface(value())
	}
}

func BenchmarkFuncWithTyped(b *testing.B) {
	for i := 0; i < b.N; i++ {
		FuncWithTyped(value())
	}
}
