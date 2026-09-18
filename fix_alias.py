import os

for fname in ['http.go', 'request.go', 'response.go']:
    fpath = os.path.join(r'd:\CodingProjects\mach\proto\http', fname)
    with open(fpath, 'r', encoding='utf-8') as f:
        text = f.read()

    if 'machcompress.' in text and 'machcompress "' not in text:
        # insert import
        text = text.replace('import (', 'import (\n\tmachcompress "github.com/lemon4ksan/mach/proto/compress"\n', 1)

        with open(fpath, 'w', encoding='utf-8') as f:
            f.write(text)
