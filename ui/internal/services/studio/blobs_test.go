package studio

import "testing"

func TestStashHoldsBytesByContentAndLetsGoOfTheOldestOverItsLimit(t *testing.T) {
	s := newStash(10)
	a := s.Put([]byte("aaaa"))
	if again := s.Put([]byte("aaaa")); again != a {
		t.Error("the same bytes were given two names")
	}
	if got, ok := s.Get(a); !ok || string(got) != "aaaa" {
		t.Fatalf("got %q, %v", got, ok)
	}
	b := s.Put([]byte("bbbb"))
	// Reading a makes it the newest, so b is the one to go
	s.Get(a)
	c := s.Put([]byte("cccc"))
	if _, ok := s.Get(b); ok {
		t.Error("the oldest was kept over the limit")
	}
	for _, name := range []string{a, c} {
		if _, ok := s.Get(name); !ok {
			t.Errorf("%s was let go", name)
		}
	}
	// What was put in is a copy
	in := []byte("dd")
	d := s.Put(in)
	in[0] = 'x'
	if got, _ := s.Get(d); string(got) != "dd" {
		t.Errorf("held %q", got)
	}
	// A single file over the limit is still held, so the edit that made it can be undone
	big := s.Put(make([]byte, 50))
	if _, ok := s.Get(big); !ok {
		t.Error("a file over the limit was not held")
	}
}
