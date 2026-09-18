import os
import re

fpath = r'd:\CodingProjects\mach\proto\headers\header.go'
with open(fpath, 'r', encoding='utf-8') as f:
    text = f.read()

# Remove VisitAll function
text = re.sub(r'// VisitAll invokes the f callback.*?\nfunc \(h \*Headers\) VisitAll\(f func\(key, value string\)\) \{.*?\n\s*\}\n', '', text, flags=re.MULTILINE|re.DOTALL)

with open(fpath, 'w', encoding='utf-8') as f:
    f.write(text)

