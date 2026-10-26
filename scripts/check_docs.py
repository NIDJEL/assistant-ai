#!/usr/bin/env python3
"""Check committed documentation links and reproducible submission assets (stdlib)."""
import hashlib
from html.parser import HTMLParser
import json
from pathlib import Path
import re
import struct
import subprocess
import sys
from urllib.parse import unquote, urlsplit
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]


def main():
    tracked = subprocess.check_output(['git', 'ls-files', '-z'], cwd=ROOT).decode().split('\0')
    errors = []
    documents = [ROOT / name for name in tracked if name.endswith('.md') and (ROOT / name).is_file()]
    links = 0
    for path in documents:
        text = re.sub(r'^(```|~~~).*?^\1[^\n]*$', '', path.read_text(encoding='utf-8'), flags=re.M | re.S)
        for match in re.finditer(r'!?\[[^\]\n]*\]\(([^\n]+?)\)', text):
            target = match[1].strip()
            if target.startswith('<'):
                target = target[1:target.index('>')]
            else:
                target = re.split(r'\s+[\"\']', target)[0]
            parsed = urlsplit(target)
            if parsed.scheme or parsed.netloc or not parsed.path:
                continue
            links += 1
            resolved = (path.parent / unquote(parsed.path)).resolve()
            if not resolved.exists():
                errors.append(f'{path.relative_to(ROOT)}: missing {target}')

    class Gallery(HTMLParser):
        def handle_starttag(self, tag, attrs):
            for name, value in attrs:
                if name in ('href', 'src') and value and value.startswith('diagrams/'):
                    if not (ROOT / 'docs' / value).is_file():
                        errors.append('architecture.html: missing ' + value)

    Gallery().feed((ROOT / 'docs/architecture.html').read_text(encoding='utf-8'))
    diagrams = list((ROOT / 'docs/diagrams').glob('*.mmd'))
    for source in diagrams:
        svg = source.with_suffix('.svg')
        try:
            if ET.parse(svg).getroot().tag != '{http://www.w3.org/2000/svg}svg':
                raise ValueError('not an SVG')
        except (OSError, ET.ParseError, ValueError) as error:
            errors.append(f'{svg.name}: {error}')

    directory = ROOT / 'docs/screenshots/current'
    manifest = json.loads((directory / 'manifest.json').read_text(encoding='utf-8'))
    screenshots = manifest['screenshots']
    for item in screenshots:
        path = directory / item['file']
        try:
            raw = path.read_bytes()
            width, height = struct.unpack('>II', raw[16:24])
            if raw[:8] != b'\x89PNG\r\n\x1a\n':
                raise ValueError('not a PNG')
            if hashlib.sha256(raw).hexdigest() != item['sha256']:
                raise ValueError('manifest checksum differs')
            viewport = item['viewport']
            if (width, height) != (viewport['width'], viewport['height']):
                raise ValueError('manifest viewport differs')
        except (OSError, ValueError, struct.error) as error:
            errors.append(f'{path.name}: {error}')
    actual = {p.name for p in directory.glob('*.png')}
    if actual != {item['file'] for item in screenshots}:
        errors.append('Screenshot manifest does not match the PNG files')
    if errors:
        print('\n'.join(errors), file=sys.stderr)
        return 1
    print(f'Docs: {len(documents)} Markdown files, {links} local links, {len(diagrams)} SVG diagrams, {len(screenshots)} screenshot checksums: OK')
    return 0


if __name__ == '__main__':
    sys.exit(main())
