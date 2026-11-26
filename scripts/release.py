"""Deterministic six-platform packages, completeness gates and pinned CPA downloads."""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import tarfile
import urllib.request
import zipfile

spec=importlib.util.spec_from_file_location('binary',Path(__file__).with_name('binary.py'))
binary=importlib.util.module_from_spec(spec);spec.loader.exec_module(binary)
ROOT=Path(__file__).resolve().parents[1]
ID='quota-first-router'
REPOSITORY='kazimshah39/quota-first-router'
CPA_VERSION='8.1.0'
TARGETS=tuple((os,arch) for os in ('darwin','linux','windows') for arch in ('amd64','arm64'))
CPA_HASHES={
 ('darwin','amd64'):'53534ef90ae261cf4daa518164a743174ed7eb752a7d2a79bfb35a175241c126',
 ('darwin','arm64'):'b8c45266738972bdb109f3484a3b8e8af9d70c5fa72abe08c832a65b1d9c6421',
 ('linux','amd64'):'09492d9a338eb2d84b5e86f5456af0ff1fa951ad266c3df78e7db571e8ec41b1',
 ('linux','arm64'):'614b264d97eb0ca8e7ad39ba327847191aa0c43a2537042ffad5af66687e77a7',
 ('windows','amd64'):'1497d7dea44e2097c0bbe9c8fa37f99a9b84efcc3485369ee3e7b5a504cd4ea3',
 ('windows','arm64'):'6c26406c64bc27d6d0d14c8001de289d817268503bff4463985da497e1e58e3d'}
CPA_ARCHIVE_SHA256=CPA_HASHES['linux','amd64']
def version():return re.search(r'const pluginVersion = "([0-9]+\.[0-9]+\.[0-9]+)"',(ROOT/'config.go').read_text())[1]
def extension(goos):return {'linux':'.so','darwin':'.dylib','windows':'.dll'}[goos]
def archive_name(goos,goarch):return f'{ID}_{version()}_{goos}_{goarch}.zip'
def target_from_name(path):
 match=re.fullmatch(re.escape(ID)+r'_\d+\.\d+\.\d+_(darwin|linux|windows)_(amd64|arm64)\.zip',Path(path).name)
 assert match, 'Invalid archive name';return match.groups()
def download(url,path):
 path=Path(path);path.parent.mkdir(parents=True,exist_ok=True)
 with urllib.request.urlopen(urllib.request.Request(url,headers={'User-Agent':ID+'-release-check'}),timeout=90) as response:path.write_bytes(response.read())
def checksum_entries(path):
 entries={}
 for line in Path(path).read_text().splitlines():
  digest,name=line.split();name=name.lstrip('*')
  assert re.fullmatch('[0-9a-f]{64}',digest) and name not in entries and Path(name).name==name,'Invalid/duplicate checksum entry'
  entries[name]=digest
 return entries
def verify_archive(path,checksums):
 assert hashlib.sha256(Path(path).read_bytes()).hexdigest()==checksum_entries(checksums)[Path(path).name],'Checksum mismatch'
def validate_package(path,goos=None,goarch=None):
 if goos is None or goarch is None:goos,goarch=target_from_name(path)
 expected={ID+extension(goos),'LICENSE','THIRD_PARTY_NOTICES.md'}
 with zipfile.ZipFile(path) as archive:
  assert set(archive.namelist())==expected and len(archive.namelist())==3,'Unexpected ZIP paths'
  for name in expected:assert '/' not in name and archive.read(name),'Invalid or empty asset'
  return binary.inspect(archive.read(ID+extension(goos)),goos,goarch)
def write_checksums(destination):
 archives=sorted(Path(destination).glob('*.zip'))
 (Path(destination)/'checksums.txt').write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n' for p in archives))
def package(goos='linux',goarch='amd64',library=None,output=None):
 assert (goos,goarch) in TARGETS,'Unsupported target'
 destination=Path(output or ROOT/'dist/release');destination.mkdir(parents=True,exist_ok=True)
 library=Path(library or ROOT/'dist'/(ID+extension(goos)))
 info=binary.inspect(library.read_bytes(),goos,goarch)
 path=destination/archive_name(goos,goarch)
 with zipfile.ZipFile(path,'w',compression=zipfile.ZIP_DEFLATED,compresslevel=9) as archive:
  for name,source in [(ID+extension(goos),library),('LICENSE',ROOT/'LICENSE'),('THIRD_PARTY_NOTICES.md',ROOT/'THIRD_PARTY_NOTICES.md')]:
   entry=zipfile.ZipInfo(name,(2026,1,1,0,0,0));entry.compress_type=zipfile.ZIP_DEFLATED;entry.external_attr=(0o100755 if name==ID+extension(goos) else 0o100644)<<16;archive.writestr(entry,source.read_bytes())
 write_checksums(destination);validate_package(path);verify_archive(path,destination/'checksums.txt');print('PASS package:',path,json.dumps(info));return path

def matrix(directory,require_native=False):
 directory=Path(directory);expected={archive_name(*t) for t in TARGETS}
 actual={p.name for p in directory.glob('*.zip')};assert actual==expected,'Expected exactly six version-matched target archives'
 entries=checksum_entries(directory/'checksums.txt');assert set(entries)==expected,'Incomplete/extra checksum matrix'
 assert list(entries)==sorted(entries),'Checksum matrix must be sorted'
 result=[]
 for target in TARGETS:
  p=directory/archive_name(*target);verify_archive(p,directory/'checksums.txt');info=validate_package(p);info['archive_sha256']=entries[p.name]
  if require_native:
   evidence=json.loads((directory/(f'native-{target[0]}-{target[1]}.json')).read_text())
   with zipfile.ZipFile(p) as z:digest=hashlib.sha256(z.read(ID+extension(target[0]))).hexdigest()
   assert evidence['native'] is True and evidence['goos']==target[0] and evidence['goarch']==target[1] and evidence['library_sha256']==digest and evidence['cpa_version']==CPA_VERSION and evidence['passed'] is True and evidence['local_store_install'] is True and evidence['archive_sha256']==entries[p.name] and evidence['soak_seconds']>=65 and evidence['invalid_library_rejected'] is True and evidence['wrong_arch_library_rejected'] is True and re.fullmatch('[0-9a-f]{64}',evidence['cpa_sha256']) and evidence['host_machine'] and evidence['go_version'],'Missing matching native acceptance'
  result.append(info)
 return result

def cpa(goos='linux',goarch='amd64',output=None):
 target=(goos,goarch);assert target in TARGETS
 destination=Path(output or ROOT/'dist/cpa');destination.mkdir(parents=True,exist_ok=True)
 arch='aarch64' if goarch=='arm64' else goarch;suffix='zip' if goos=='windows' else 'tar.gz';name=f'CLIProxyAPI_{CPA_VERSION}_{goos}_{arch}.{suffix}';archive=destination/name;base=f'https://github.com/router-for-me/CLIProxyAPI/releases/download/v{CPA_VERSION}/'
 download(base+name,archive);download(base+'checksums.txt',destination/'checksums.txt');verify_archive(archive,destination/'checksums.txt');assert hashlib.sha256(archive.read_bytes()).hexdigest()==CPA_HASHES[target],'CPA differs from pinned upstream checksum'
 exe='cli-proxy-api.exe' if goos=='windows' else 'cli-proxy-api';binary_path=destination/exe
 if goos=='windows':
  with zipfile.ZipFile(archive) as z:
   names=[n for n in z.namelist() if Path(n).name==exe];assert len(names)==1;binary_path.write_bytes(z.read(names[0]))
 else:
  with tarfile.open(archive) as tar:
   members=[m for m in tar.getmembers() if m.isfile() and Path(m.name).name==exe];assert len(members)==1;binary_path.write_bytes(tar.extractfile(members[0]).read())
 binary_path.chmod(0o755);print('PASS downloaded and checksum-verified CPA:',binary_path)

def main():
 p=argparse.ArgumentParser();p.add_argument('command',choices=['version','package','cpa','verify','matrix']);p.add_argument('--goos',default='linux');p.add_argument('--goarch',default='amd64');p.add_argument('--library');p.add_argument('--output');p.add_argument('--archive');p.add_argument('--checksums');p.add_argument('--require-native',action='store_true');args=p.parse_args()
 if args.command=='version':print(version())
 elif args.command=='package':package(args.goos,args.goarch,args.library,args.output)
 elif args.command=='cpa':cpa(args.goos,args.goarch,args.output)
 elif args.command=='matrix':print(json.dumps(matrix(args.output or ROOT/'dist/release',args.require_native),indent=2))
 else:verify_archive(args.archive,args.checksums);print(json.dumps(validate_package(args.archive)))
if __name__=='__main__':main()
