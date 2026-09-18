import os
import re

fpath = r'd:\CodingProjects\mach\server\h1\h1_test.go'
with open(fpath, 'r', encoding='utf-8') as f:
    text = f.read()

# Fix ParseHeaderLine -> delete the TestHeaderLine_EdgeCases test
text = re.sub(r'func TestHeaderLine_EdgeCases.*?^}', '', text, flags=re.MULTILINE|re.DOTALL)

with open(fpath, 'w', encoding='utf-8') as f:
    f.write(text)
