package lru

import (
	"reflect"
	"testing"
)

type String string

func (d String) Len() int {
	return len(d)
}

func TestGet(t *testing.T) {
	lru := New(int64(0), nil)
	lru.Add("key1", String("1234"), int64(len("1234")))
	if v, ok := lru.Get("key1"); !ok || string(v.(String)) != "1234" {
		t.Fatalf("cache hit key1=1234 failed")
	}
	if _, ok := lru.Get("key2"); ok {
		t.Fatalf("cache miss key2 failed")
	}
}

func TestRemoveoldest(t *testing.T) {
	k1, k2, k3 := "key1", "key2", "k3"
	v1, v2, v3 := "value1", "value2", "v3"
	cap := len(k1 + k2 + v1 + v2)
	lru := New(int64(cap), nil)
	lru.Add(k1, String(v1), int64(len(v1)))
	lru.Add(k2, String(v2), int64(len(v2)))
	lru.Add(k3, String(v3), int64(len(v3)))

	if _, ok := lru.Get("key1"); ok || lru.Len() != 2 {
		t.Fatalf("Removeoldest key1 failed")
	}
}

func TestOnEvicted(t *testing.T) {
	keys := make([]string, 0)
	callback := func(key string, value Value) {
		keys = append(keys, key)
	}
	lru := New(int64(10), callback)
	lru.Add("key1", String("123456"), int64(len("123456")))
	lru.Add("k2", String("k2"), int64(len("k2")))
	lru.Add("k3", String("k3"), int64(len("k3")))
	lru.Add("k4", String("k4"), int64(len("k4")))

	expect := []string{"key1", "k2"}

	if !reflect.DeepEqual(expect, keys) {
		t.Fatalf("Call OnEvicted failed, expect keys equals to %s", expect)
	}
}

func TestAdd(t *testing.T) {
	lru := New(int64(0), nil)
	lru.Add("key", String("1"), int64(len("1")))
	lru.Add("key", String("111"), int64(len("111")))

	if lru.nbytes != int64(len("key")+len("111")) {
		t.Fatal("expected 6 but got", lru.nbytes)
	}
}
