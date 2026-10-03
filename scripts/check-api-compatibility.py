#!/usr/bin/env python3
"""Conservative compatibility gate for this repository's OpenAPI contract."""
import json
import subprocess
import sys
from pathlib import Path

base = sys.argv[1] if len(sys.argv) > 1 else 'origin/main'
subprocess.run(['git', 'rev-parse', '--verify', base], check=True, stdout=subprocess.DEVNULL)
files = subprocess.check_output(['git', 'ls-tree', '-r', '--name-only', base], text=True).splitlines()
if 'docs/openapi.json' not in files:
    print('Base revision has no OpenAPI contract; establishing first baseline.')
    sys.exit(0)
old = json.loads(subprocess.check_output(['git', 'show', base + ':docs/openapi.json'], text=True))
new = json.loads(Path('docs/openapi.json').read_text())
errors = []

def compare(previous, current, location):
    if not isinstance(previous, dict) or not isinstance(current, dict):
        return
    for key in ['$ref', 'type', 'format', 'const']:
        if key in previous and previous[key] != current.get(key):
            errors.append(f'{location}: changed {key}')
    if 'enum' in previous and not set(previous['enum']) <= set(current.get('enum', previous['enum'])):
        errors.append(f'{location}: narrowed enum')
    for key in ['minimum', 'minLength', 'minItems']:
        if key in current and current[key] > previous.get(key, float('-inf')):
            errors.append(f'{location}: tightened {key}')
    for key in ['maximum', 'maxLength', 'maxItems']:
        if key in current and current[key] < previous.get(key, float('inf')):
            errors.append(f'{location}: tightened {key}')
    if set(current.get('required', [])) - set(previous.get('required', [])):
        errors.append(f'{location}: added required fields')
    for name, schema in previous.get('properties', {}).items():
        if name not in current.get('properties', {}):
            errors.append(f'{location}: removed property {name}')
        else:
            compare(schema, current['properties'][name], location + '.' + name)
    if 'items' in previous:
        compare(previous['items'], current.get('items', {}), location + '[]')
    for key in ['oneOf', 'anyOf', 'allOf']:
        if key in previous and previous[key] != current.get(key):
            errors.append(f'{location}: changed {key}; requires compatibility review')

for path, methods in old['paths'].items():
    for method, operation in methods.items():
        current = new.get('paths', {}).get(path, {}).get(method)
        if current is None:
            errors.append(f'{method} {path}: removed operation')
            continue
        params = {(p['in'], p['name']): p for p in current.get('parameters', [])}
        old_params = {(p['in'], p['name']): p for p in operation.get('parameters', [])}
        for key, param in old_params.items():
            if key not in params:
                errors.append(f'{method} {path}: removed parameter {key}')
            else:
                compare(param.get('schema', {}), params[key].get('schema', {}), f'{path} parameter {key}')
                if params[key].get('required') and not param.get('required'):
                    errors.append(f'{path}: made parameter {key} required')
        for key, param in params.items():
            if key not in old_params and param.get('required'):
                errors.append(f'{path}: added required parameter {key}')
        for status, response in operation.get('responses', {}).items():
            if status not in current.get('responses', {}):
                errors.append(f'{path}: removed response {status}')
            else:
                for media, body in response.get('content', {}).items():
                    other = current['responses'][status].get('content', {}).get(media)
                    if other is None:
                        errors.append(f'{path}: removed response content {media}')
                    else:
                        compare(body.get('schema', {}), other.get('schema', {}), f'{path} response {status}')
for name, schema in old.get('components', {}).get('schemas', {}).items():
    current = new.get('components', {}).get('schemas', {}).get(name)
    if current is None:
        errors.append(f'removed schema {name}')
    else:
        compare(schema, current, name)
if errors:
    print('\n'.join(errors), file=sys.stderr)
    sys.exit(1)
print('No incompatible changes detected by the conservative contract gate.')
