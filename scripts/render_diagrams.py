#!/usr/bin/env python3
"""Render the checked-in Mermaid sources. Optional: Node 24 + npm; not needed to run the app."""
import argparse
import os
from pathlib import Path
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--browser', help='Optional Chrome/Chromium executable; avoids a separate browser download')
    parser.add_argument('names', nargs='*', help='Optional diagram stems; default: all diagrams')
    args = parser.parse_args()
    npx = shutil.which('npx')
    if not npx:
        raise RuntimeError('Node.js 24 and npm/npx are required only to regenerate diagrams.')
    env = dict(os.environ)
    if args.browser:
        env.update(PUPPETEER_EXECUTABLE_PATH=str(Path(args.browser).resolve()), PUPPETEER_SKIP_DOWNLOAD='true')
    sources = sorted((ROOT / 'docs/diagrams').glob('*.mmd'))
    if args.names:
        sources = [source for source in sources if source.stem in args.names]
        if len(sources) != len(set(args.names)):
            raise ValueError('Unknown diagram name')
    for source in sources:
        subprocess.run([npx, '--yes', '--package', '@mermaid-js/mermaid-cli@12.0.0', 'mmdc',
                        '-i', str(source), '-o', str(source.with_suffix('.svg')),
                        '-c', str(ROOT / 'docs/diagrams/mermaid-config.json'), '-b', 'white', '--size', '1800', '--no-font-embed'],
                       cwd=ROOT, env=env, check=True)
        print('Rendered ' + source.name)


if __name__ == '__main__':
    try:
        main()
    except (ValueError, RuntimeError, subprocess.CalledProcessError) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
