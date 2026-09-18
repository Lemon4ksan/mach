import os
import re

for d in [r'd:\CodingProjects\mach', r'd:\CodingProjects\aoni']:
    for root, _, files in os.walk(d):
        if '.git' in root: continue
        for f in files:
            if not f.endswith('.go'): continue
            try:
                with open(os.path.join(root, f), 'r', encoding='utf-8') as file:
                    content = file.read()
                    if re.search(r'type\s+[mM]ultiError\s+(struct|\[\]error)', content):
                        print(f"Found in {os.path.join(root, f)}")
            except: pass
