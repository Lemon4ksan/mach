import os
import re

fpath = r'd:\CodingProjects\mach\server\h1\h1_test.go'
with open(fpath, 'r', encoding='utf-8') as f:
    text = f.read()

# Fix zerocopy.Cookie struct literal
text = re.sub(r'zerocopy\.Cookie\{\s*Name:\s*([^,]+),\s*Value:\s*([^,]+),\s*Path:\s*([^,]+),?\s*\}', 
              r'createTestCookie(\1, \2, \3)', text)

# Add createTestCookie helper
if 'func createTestCookie' not in text:
    text += "\nfunc createTestCookie(name, value, path []byte) *zerocopy.Cookie {\n\tc := zerocopy.AcquireCookie()\n\tc.SetKeyBytes(name)\n\tc.SetValueBytes(value)\n\tc.SetPathBytes(path)\n\treturn c\n}\n"

# Fix ParseHeaderLine
text = text.replace('h.ParseHeaderLine(', 'h1.ParseHeaderLine(h, ')
# wait, ParseHeaderLine isn't a method on headers.Headers anymore, but wait! Does h1.ParseHeaderLine exist? Let's check header.go in a bit. Or just comment out that test if it's too tied to internals.

with open(fpath, 'w', encoding='utf-8') as f:
    f.write(text)
