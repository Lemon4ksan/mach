import os

fpath = r'C:\Users\senya\.gemini\antigravity\brain\fc2bb0cf-71ee-4f38-92aa-64b50dc6a2e9\task.md'
with open(fpath, 'r', encoding='utf-8') as f:
    text = f.read()

text = text.replace('- [ ] **3. Standardize Structured Logging', '- [x] **3. Standardize Structured Logging')
text = text.replace('- [ ] **4. Modern Multi-Errors', '- [x] **4. Modern Multi-Errors')
with open(fpath, 'w', encoding='utf-8') as f:
    f.write(text)
