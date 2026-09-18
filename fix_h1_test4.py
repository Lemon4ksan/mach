import os

fpath = r'd:\CodingProjects\mach\server\h1\h1_test.go'
with open(fpath, 'r', encoding='utf-8') as f:
    text = f.read()

text = text.replace('"Set-Cookie: token=secret123; Path=/\\r\\n"', '"Set-Cookie: token=secret123; path=/\\r\\n"')

with open(fpath, 'w', encoding='utf-8') as f:
    f.write(text)
