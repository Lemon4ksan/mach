import os
import re

for fname in ['sys_conn.go', 'sys_conn_oob.go']:
    fpath = os.path.join(r'd:\CodingProjects\mach\quic', fname)
    with open(fpath, 'r', encoding='utf-8') as f:
        text = f.read()

    text = text.replace('log.Printf(', 'slog.Warn(')
    text = text.replace('"log"', '"log/slog"\n\t"log"')

    with open(fpath, 'w', encoding='utf-8') as f:
        f.write(text)

