import os
import re

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

if 'func (h *Headers) All()' not in text:
    text = text.replace('func (h *Headers) VisitAll', iterator_code + '\nfunc (h *Headers) VisitAll')

with open(fpath, 'w', encoding='utf-8') as f:
    f.write(text)
