import os
import re

fpath = r'd:\CodingProjects\mach\proto\h3\qpack.go'
with open(fpath, 'r', encoding='utf-8') as f:
    text = f.read()

# Replace headers.VisitAll(func(k, v string) { ... }) with for k, v := range headers.All() { ... }
# We need to be careful with multi-line replacements.
# In qpack.go:456:
#	headers.VisitAll(func(k, v string) {
#		if !machhttp.IsValidHeaderValue(v) {
#			return // skip in VisitAll
#       }
#       ...
#   })
text = re.sub(
    r'headers\.VisitAll\(func\(k,\s*v\s*string\)\s*\{',
    r'for k, v := range headers.All() {',
    text
)
text = text.replace('return // skip in VisitAll', 'continue // skip in VisitAll')
# But wait, we need to remove the closing }) of the VisitAll! 
# Let's just do it manually with python split.
