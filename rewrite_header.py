import os

fpath = r'd:\CodingProjects\mach\proto\headers\header.go'
with open(fpath, 'r', encoding='utf-8') as f:
    text = f.read()

# Add iter import
if '"iter"' not in text:
    text = text.replace('import (', 'import (\n\t"iter"\n')

iterator_code = """
// All returns an iterator over all headers.
func (h *Headers) All() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		for i := 0; i < h.count; i++ {
			p := h.packed[i]
			if p != 0 {
				if !yield(h.getKey(p), h.getVal(p)) {
					return
				}
			}
		}
		for i := 0; i < len(h.dynamic); i++ {
			if !yield(h.dynamic[i].Key, h.dynamic[i].Value) {
				return
			}
		}
	}
}
"""

# Replace VisitAll entirely
visit_all_legacy = """// VisitAll invokes the f callback for each header without allocating a slice
func (h *Headers) VisitAll(f func(key, value string)) {
	for i := 0; i < h.count; i++ {
		p := h.packed[i]
		if p != 0 {
			f(h.getKey(p), h.getVal(p))
		}
	}

	for i := 0; i < len(h.dynamic); i++ {
		f(h.dynamic[i].Key, h.dynamic[i].Value)
	}
}"""
text = text.replace(visit_all_legacy, iterator_code)

# Replace Entries rewrite
entries_legacy = """func (h *Headers) Entries() []HeaderEntry {
	entries := make([]HeaderEntry, 0, h.count+len(h.dynamic))
	h.VisitAll(func(k, v string) {
		entries = append(entries, HeaderEntry{Key: k, Value: v})
	})

	return entries
}"""
entries_new = """func (h *Headers) Entries() []HeaderEntry {
	entries := make([]HeaderEntry, 0, h.count+len(h.dynamic))
	for k, v := range h.All() {
		entries = append(entries, HeaderEntry{Key: k, Value: v})
	}

	return entries
}"""
text = text.replace(entries_legacy, entries_new)

with open(fpath, 'w', encoding='utf-8') as f:
    f.write(text)
