import os
import re

fpath = r'd:\CodingProjects\mach\client\h2\conn.go'
with open(fpath, 'r', encoding='utf-8') as f:
    text = f.read()

text = text.replace('nextID:     1,', '')
# add c.nextID.Store(1) after c := &conn{...}
text = re.sub(r'(c := &Conn(?:.*\n)*?\s*\})\n', r'\1\n\tc.nextID.Store(1)\n', text)

with open(fpath, 'w', encoding='utf-8') as f:
    f.write(text)
