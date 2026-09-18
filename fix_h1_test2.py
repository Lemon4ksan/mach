import os
import re

fpath = r'd:\CodingProjects\mach\server\h1\h1_test.go'
with open(fpath, 'r', encoding='utf-8') as f:
    text = f.read()

# Fix string constants -> bytes
text = text.replace('createTestCookie("token", "secret123", "/")', 'createTestCookie([]byte("token"), []byte("secret123"), []byte("/"))')

# Fix taking address of createTestCookie
text = text.replace('&createTestCookie', 'createTestCookie')

# Fix ParseHeaderLine -> we just delete the TestParseHeaderLine test because it was testing a deleted function
text = re.sub(r'func TestParseHeaderLine.*?^}', '', text, flags=re.MULTILINE|re.DOTALL)

with open(fpath, 'w', encoding='utf-8') as f:
    f.write(text)
