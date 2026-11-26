"""Validate and extract ONLY the platform library for native acceptance."""
import argparse
import importlib.util
from pathlib import Path
import zipfile
spec=importlib.util.spec_from_file_location('release',Path(__file__).with_name('release.py'));r=importlib.util.module_from_spec(spec);spec.loader.exec_module(r)
p=argparse.ArgumentParser();p.add_argument('--goos',required=True);p.add_argument('--goarch',required=True);a=p.parse_args();archive=r.ROOT/'dist/release'/r.archive_name(a.goos,a.goarch);r.verify_archive(archive,archive.parent/'checksums.txt');r.validate_package(archive,a.goos,a.goarch);name=r.ID+r.extension(a.goos);target=r.ROOT/'dist/native'/name;target.parent.mkdir(parents=True,exist_ok=True)
with zipfile.ZipFile(archive) as z:target.write_bytes(z.read(name))
print(target)
