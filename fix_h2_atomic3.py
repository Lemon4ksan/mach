import os
import re

fpath = r'd:\CodingProjects\mach\client\h2\conn.go'
with open(fpath, 'r', encoding='utf-8') as f:
    text = f.read()

text = re.sub(r'nextID:\s+1,', '', text)
text = text.replace('c.nextID.Store(1)', 'nc.nextID.Store(1)')

with open(fpath, 'w', encoding='utf-8') as f:
    f.write(text)
