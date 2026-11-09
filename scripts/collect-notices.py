#!/usr/bin/env python3
"""Collect verbatim installed dependency license files for source distribution."""
import json
from pathlib import Path
import subprocess

root = Path(__file__).resolve().parent.parent
parts = ['# Third-party notices\n\nCodeXArtifacts uses the packages below under their own licenses.\nThe root MIT license does not replace these terms.\n\nBundled artifact runtime notices are also preserved in `internal/jsruntime/v1/NOTICES.txt`.\nFont licenses are retained next to their files in `internal/web/static/fonts`,\n`website/fonts` and `docs/licenses/fonts`.\n']
missing = []

def append_package(name, version, directory, declared=''):
    directory = Path(directory)
    files = sorted(p for p in directory.iterdir() if p.is_file() and p.name.lower().startswith(('license', 'licence', 'copying', 'notice', 'copyright')))
    parts.append('\n## ' + name + ' ' + version + '\n\n')
    if declared:
        parts.append('Declared license: ' + str(declared) + '\n\n')
    if not files:
        missing.append(name + '@' + version)
        parts.append('No standalone license file in the installed package. See its declared license and upstream source.\n')
    for file in files:
        text = file.read_text(errors='replace')
        parts.append('### ' + file.name + '\n\n```text\n' + text.rstrip() + '\n```\n')

seen = set()
for workspace in ('web', 'runtime'):
    result = subprocess.run([str(root / '.tools/node/bin/npm'), 'ls', '--omit=dev', '--all', '--parseable'], cwd=root/workspace, capture_output=True, text=True)
    for line in result.stdout.splitlines():
        path = Path(line)
        if path == root/workspace or not (path/'package.json').is_file():
            continue
        pkg = json.loads((path/'package.json').read_text())
        key = (pkg['name'], pkg['version'])
        if key in seen:
            continue
        seen.add(key)
        append_package(*key, path, pkg.get('license', ''))

result = subprocess.run(['go', 'list', '-deps', '-json', './cmd/codexartifacts'], cwd=root, capture_output=True, text=True, check=True)
decoder = json.JSONDecoder()
remaining = result.stdout.lstrip()
seen_go = set()
while remaining:
    module, end = decoder.raw_decode(remaining)
    remaining = remaining[end:].lstrip()
    module = module.get('Module', {})
    if not module or module.get('Path') in seen_go:
        continue
    seen_go.add(module['Path'])
    if module.get('Main'):
        continue
    if 'Dir' not in module:
        raise SystemExit('Download module before collecting notices: ' + module['Path'])
    append_package(module['Path'], module.get('Version', ''), module['Dir'])

parts.append('\n## Supplemental upstream licenses and font notices\n\nSome npm tarballs omit license files. The upstream license texts and bundled-font notices are preserved here and in docs/licenses. The Arimo OFL fallback replaces the legacy Liberation font binary.\n')
for file in sorted((root/'docs/licenses').rglob('*.txt')):
    parts.append('\n### ' + str(file.relative_to(root)) + '\n\n```text\n' + file.read_text().rstrip() + '\n```\n')
parts.append('\n### Arimo fallback\n\n```text\n' + (root/'web/fonts/OFL-Arimo.txt').read_text().rstrip() + '\n```\n')
output = ''.join(parts)
(root/'THIRD_PARTY_NOTICES.md').write_text(output)
(root/'internal/web/static/third-party-notices.txt').write_text(output)
print('Collected', len(seen), 'npm packages and Go module notices.')
print('Packages without standalone license files:', ', '.join(missing))
