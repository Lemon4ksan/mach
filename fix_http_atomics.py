import os
import re

for fname in ['request.go', 'response.go']:
    fpath = os.path.join(r'd:\CodingProjects\mach\proto\http', fname)
    with open(fpath, 'r', encoding='utf-8') as f:
        text = f.read()

    text = text.replace('atomic.LoadInt64(&requestBodyPoolSizeLimit)', 'requestBodyPoolSizeLimit.Load()')
    text = text.replace('atomic.LoadInt64(&responseBodyPoolSizeLimit)', 'responseBodyPoolSizeLimit.Load()')

    with open(fpath, 'w', encoding='utf-8') as f:
        f.write(text)

fpath = r'd:\CodingProjects\mach\proto\http\http.go'
with open(fpath, 'r', encoding='utf-8') as f:
    text = f.read()

text = text.replace('requestBodyPoolSizeLimit  int64 = -1', 'requestBodyPoolSizeLimit  atomic.Int64')
text = text.replace('responseBodyPoolSizeLimit int64 = -1', 'responseBodyPoolSizeLimit atomic.Int64')
text = text.replace('atomic.StoreInt64(&requestBodyPoolSizeLimit, int64(reqBodyLimit))', 'requestBodyPoolSizeLimit.Store(int64(reqBodyLimit))')
text = text.replace('atomic.StoreInt64(&responseBodyPoolSizeLimit, int64(respBodyLimit))', 'responseBodyPoolSizeLimit.Store(int64(respBodyLimit))')

# We need to initialize them to -1. Let's add an init() function or just wait if they are initialized somewhere else.
text = text + "\nfunc init() {\n\trequestBodyPoolSizeLimit.Store(-1)\n\tresponseBodyPoolSizeLimit.Store(-1)\n}\n"

with open(fpath, 'w', encoding='utf-8') as f:
    f.write(text)
