"""Build-independent release packaging and CPA download helpers (stdlib only)."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import tarfile
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parents[1]
ID = 'quota-first-router'
REPOSITORY = 'kazimshah39/quota-first-router'
CPA_VERSION = '8.1.0'
CPA_ARCHIVE_SHA256 = '09492d9a338eb2d84b5e86f5456af0ff1fa951ad266c3df78e7db571e8ec41b1'


def version():
    return re.search(r'const pluginVersion = "([0-9]+\.[0-9]+\.[0-9]+)"', (ROOT / 'config.go').read_text())[1]


def download(url, path):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    request = urllib.request.Request(url, headers={'User-Agent': ID + '-release-check'})
    with urllib.request.urlopen(request, timeout=90) as response:
        path.write_bytes(response.read())


def verify_archive(path, checksums):
    entries = {}
    for line in Path(checksums).read_text().splitlines():
        digest, name = line.split()
        entries[name.lstrip('*')] = digest
    assert hashlib.sha256(Path(path).read_bytes()).hexdigest() == entries[Path(path).name], 'Checksum mismatch'


def validate_package(path):
    expected = {ID + '.so', 'LICENSE', 'THIRD_PARTY_NOTICES.md'}
    with zipfile.ZipFile(path) as archive:
        assert set(archive.namelist()) == expected and len(archive.namelist()) == 3, 'Unexpected ZIP paths'
        library = archive.read(ID + '.so')
        assert library[:5] == b'\x7fELF\x02' and library[18:20] == b'\x3e\x00', 'Expected 64-bit x86 ELF'
        for name in expected:
            assert '/' not in name and archive.read(name), 'Invalid or empty asset'


def package():
    destination = ROOT / 'dist' / 'release'
    destination.mkdir(parents=True, exist_ok=True)
    archive_path = destination / f'{ID}_{version()}_linux_amd64.zip'
    with zipfile.ZipFile(archive_path, 'w', compression=zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
        for name, path in [(ID + '.so', ROOT / 'dist' / (ID + '.so')),
                           ('LICENSE', ROOT / 'LICENSE'), ('THIRD_PARTY_NOTICES.md', ROOT / 'THIRD_PARTY_NOTICES.md')]:
            info = zipfile.ZipInfo(name, (2026, 1, 1, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = (0o100755 if name.endswith('.so') else 0o100644) << 16
            archive.writestr(info, path.read_bytes())
    (destination / 'checksums.txt').write_text(hashlib.sha256(archive_path.read_bytes()).hexdigest() + '  ' + archive_path.name + '\n')
    validate_package(archive_path)
    verify_archive(archive_path, destination / 'checksums.txt')
    print('PASS package:', archive_path)


def cpa():
    destination = ROOT / 'dist' / 'cpa'
    archive = destination / f'CLIProxyAPI_{CPA_VERSION}_linux_amd64.tar.gz'
    checksums = destination / 'checksums.txt'
    base = f'https://github.com/router-for-me/CLIProxyAPI/releases/download/v{CPA_VERSION}/'
    download(base + archive.name, archive)
    download(base + 'checksums.txt', checksums)
    verify_archive(archive, checksums)
    assert hashlib.sha256(archive.read_bytes()).hexdigest() == CPA_ARCHIVE_SHA256, 'CPA differs from pinned upstream checksum'
    with tarfile.open(archive) as tar:
        members = [m for m in tar.getmembers() if m.isfile() and Path(m.name).name == 'cli-proxy-api']
        assert len(members) == 1
        binary = destination / 'cli-proxy-api'
        binary.write_bytes(tar.extractfile(members[0]).read())
        binary.chmod(0o755)
    print('PASS downloaded and checksum-verified CPA:', binary)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('command', choices=['version', 'package', 'cpa', 'verify'])
    parser.add_argument('--archive')
    parser.add_argument('--checksums')
    args = parser.parse_args()
    if args.command == 'version':
        print(version())
    elif args.command == 'package':
        package()
    elif args.command == 'cpa':
        cpa()
    else:
        verify_archive(args.archive, args.checksums)
        validate_package(args.archive)
        print('PASS downloaded package checksum/layout')


if __name__ == '__main__':
    main()
