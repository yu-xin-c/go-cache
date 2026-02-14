package singleflight

import (
	"testing"
)

func TestDo(t *testing.T) {
	var g Group
	v, err, shared := g.Do("key", func() (interface{}, error) {
		return "bar", nil
	})

	if v != "bar" || err != nil {
		t.Errorf("Do v = %v, error = %v", v, err)
	}

	// 第一个请求应该是 shared=false
	if shared {
		t.Errorf("Expected shared=false for first call, got shared=true")
	}
}
