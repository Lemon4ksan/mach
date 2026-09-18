import os
import re

count = 0
for d in [r'd:\CodingProjects\mach', r'd:\CodingProjects\aoni']:
    for root, _, files in os.walk(d):
        if '.git' in root: continue
        for f in files:
            if not f.endswith('.go'): continue
            try:
                with open(os.path.join(root, f), 'r', encoding='utf-8') as file:
                    count += len(re.findall(r'\bany\b', file.read()))
            except: pass
print(f"any: {count}")
