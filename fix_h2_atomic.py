import os
import re

fpath = r'd:\CodingProjects\mach\client\h2\conn.go'
with open(fpath, 'r', encoding='utf-8') as f:
    text = f.read()

# nextID uint32 -> nextID atomic.Uint32
text = re.sub(r'nextID\s+uint32', r'nextID atomic.Uint32', text)
text = re.sub(r'atomic\.AddUint32\(&c\.nextID,\s*([^\)]+)\)', r'c.nextID.Add(\1)', text)
text = re.sub(r'atomic\.LoadUint32\(&c\.nextID\)', r'c.nextID.Load()', text)

with open(fpath, 'w', encoding='utf-8') as f:
    f.write(text)
