import os
import re

fpath = r'd:\CodingProjects\mach\quic\internal\utils\log.go'
with open(fpath, 'r', encoding='utf-8') as f:
    text = f.read()

text = text.replace('log.Printf(', 'slog.Info(')
text = text.replace('"log"', '"log/slog"\n\t"fmt"')
text = text.replace('pre+format', 'fmt.Sprintf(pre+format, args...)')
text = text.replace(', args...', '')

with open(fpath, 'w', encoding='utf-8') as f:
    f.write(text)

