import os
import re

dirs_to_scan = [r'd:\CodingProjects\mach', r'd:\CodingProjects\aoni']
legacy_patterns = {
    'interface{}': re.compile(r'\binterface\{\}'),
    'ioutil': re.compile(r'\bioutil\.'),
    'legacy_atomic': re.compile(r'atomic\.(Add|Load|Store|CompareAndSwap|Swap)(Int|Uint|Pointer)'),
    'sync.Pool': re.compile(r'\bsync\.Pool\b'),
    'golang.org/x/exp/slices': re.compile(r'"golang.org/x/exp/slices"'),
    'golang.org/x/exp/maps': re.compile(r'"golang.org/x/exp/maps"')
}

counts = {k: 0 for k in legacy_patterns}

for d in dirs_to_scan:
    for root, _, files in os.walk(d):
        if '.git' in root: continue
        for f in files:
            if not f.endswith('.go'): continue
            try:
                with open(os.path.join(root, f), 'r', encoding='utf-8') as file:
                    content = file.read()
                    for k, pat in legacy_patterns.items():
                        counts[k] += len(pat.findall(content))
            except:
                pass

for k, v in counts.items():
    print(f"{k}: {v}")
