package engine

import (
	"testing"

	"github.com/liberide/serpent-seek/internal/providers"
)

func BenchmarkNormalizeLink(b *testing.B) {
	links := []string{
		"https://Example.com/path/?utm_source=x&q=1#frag",
		"https://www.example.com/a/b/",
		"http://EXAMPLE.org:8080/x?ref=abc&id=2",
		"not a url",
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = normalizeLink(links[i%len(links)])
	}
}

func BenchmarkMergerAdd(b *testing.B) {
	rows := providers.Rows{
		{Link: "https://a.example/1", Title: "a"},
		{Link: "https://a.example/1?utm_source=x", Title: "a dup"},
		{Link: "https://b.example/2", Title: "b"},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m := newMerger()
		m.add("p1", rows)
		m.add("p2", rows)
	}
}
