"""Verified public HitPipe documents and build assets for the school origin."""

from hashlib import sha256
import json
from pathlib import Path, PurePosixPath
import re

from school_gateway import SEALED_SESSION, SESSION_COOKIE


PAGES = {
    'public/hitpipe-entry.html': '/',
    'public/hitpipe-operation-entry.html': '/operation-setup',
}
STATIC_PATH = re.compile(r'public/_next/static(?:/[A-Za-z0-9_.\[\]-]+)+')
SHA256 = re.compile(r'[a-f0-9]{64}')
ASSET_TYPES = {
    '.css': 'text/css; charset=utf-8',
    '.js': 'application/javascript; charset=utf-8',
    '.json': 'application/json; charset=utf-8',
    '.map': 'application/json; charset=utf-8',
    '.png': 'image/png',
    '.svg': 'image/svg+xml',
    '.woff': 'font/woff',
    '.woff2': 'font/woff2',
}
MAX_STAGE_BYTES = 16 * 1024 * 1024


def view_path(name):
    if name in PAGES:
        return PAGES[name], 'text/html; charset=utf-8', 'private, no-store'
    if name == 'public/hitpipe-mark.svg':
        return '/hitpipe-mark.svg', ASSET_TYPES['.svg'], 'public, max-age=3600'
    if STATIC_PATH.fullmatch(name) and not ({'.', '..'} & set(name.split('/'))):
        kind = ASSET_TYPES.get(PurePosixPath(name).suffix)
        if kind:
            return name.removeprefix('public'), kind, 'public, max-age=31536000, immutable'
    return None


class GuestSnapshot:
    def __init__(self, stage, manifest, expected_sha256):
        root = Path(stage).resolve(strict=True)
        manifest = Path(manifest)
        if not SHA256.fullmatch(expected_sha256) or manifest.stat().st_size > 128 * 1024:
            raise ValueError('Invalid guest snapshot manifest')
        metadata = manifest.read_bytes()
        if sha256(metadata).hexdigest() != expected_sha256:
            raise ValueError('Guest snapshot manifest does not match the pinned digest')
        rows = json.loads(metadata)['files']
        if not isinstance(rows, list) or not 3 <= len(rows) <= 256:
            raise ValueError('Invalid number of guest snapshot files')

        files = {}
        size = 0
        for row in rows:
            name = row['path']
            mapped = view_path(name)
            if mapped is None:
                if name == 'vercel.json':
                    continue
                raise ValueError('Unexpected file in the guest snapshot')
            path, kind, cache = mapped
            file = root / name
            if file.resolve(strict=True) != file or not file.is_file() or path in files:
                raise ValueError('Invalid or duplicate guest snapshot path')
            length = file.stat().st_size
            size += length
            if size > MAX_STAGE_BYTES or length != row['bytes']:
                raise ValueError('Unexpected guest snapshot size')
            data = file.read_bytes()
            digest = sha256(data).hexdigest()
            if len(data) != length or digest != row['sha256']:
                raise ValueError('Guest asset does not match the manifest')
            files[path] = (kind, cache, f'"{digest}"', data)
        if not {*PAGES.values(), '/hitpipe-mark.svg'} <= files.keys():
            raise ValueError('Missing guest documents or logo')
        self.files = files

    def response(self, method, path, request):
        if method not in ('GET', 'HEAD') or path not in self.files:
            return None
        guest = path in PAGES.values()
        if guest and any(
            key == SESSION_COOKIE and SEALED_SESSION.fullmatch(value)
            for key, _, value in (part.strip().partition('=') for part in request.get('Cookie', '').split(';'))
        ):
            return None

        kind, cache, tag, content = self.files[path]
        headers = [('Content-Type', kind), ('Cache-Control', cache), ('X-Content-Type-Options', 'nosniff')]
        if guest:
            headers.append(('Vary', 'Cookie'))
        else:
            headers.append(('ETag', tag))
            matches = request.get('If-None-Match', '').split(',')
            if any(value.strip().removeprefix('W/') in ('*', tag) for value in matches):
                return 304, headers, b'', len(content)
        return 200, headers, content, len(content)
