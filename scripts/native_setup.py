"""Configure native cgo compiler on disposable GitHub runners (never a release action)."""
import argparse
import importlib.util
import os
from pathlib import Path
import platform
import shutil
import subprocess
import urllib.request
import zipfile

p=argparse.ArgumentParser();p.add_argument('--output',required=True);a=p.parse_args();root=Path(__file__).resolve().parents[1]
lines=[]
if platform.system()=='Windows':
 arch='aarch64' if platform.machine().lower() in ('arm64','aarch64') else 'x86_64'
 # Same source/version as pinned Linux cross compiler. Verify GitHub's published
 # content digest for the native archive before extraction; fail if absent.
 import json,hashlib
 api='https://api.github.com/repos/mstorsjo/llvm-mingw/releases/tags/20261117'
 request=urllib.request.Request(api,headers={'User-Agent':'quota-first-router-native-validation'})
 with urllib.request.urlopen(request,timeout=60) as response:release=json.load(response)
 name=f'llvm-mingw-20261117-ucrt-{arch}.zip';asset=next(v for v in release['assets'] if v['name']==name);digest=asset.get('digest','');assert digest.startswith('sha256:'),'Published compiler digest required'
 tools=root/'.local/native-tools';tools.mkdir(parents=True,exist_ok=True);archive=tools/name
 with urllib.request.urlopen(asset['browser_download_url'],timeout=120) as response,archive.open('wb') as f:shutil.copyfileobj(response,f)
 assert hashlib.sha256(archive.read_bytes()).hexdigest()==digest[7:],'Native compiler digest mismatch'
 with zipfile.ZipFile(archive) as z:
  assert all('..' not in Path(n).parts and not Path(n).is_absolute() for n in z.namelist());z.extractall(tools)
 compiler=tools/name.removesuffix('.zip')/'bin'/f'{arch}-w64-mingw32-clang.exe';assert compiler.exists()
 # CC is parsed as a shell command by cgo, so quote Windows paths.
 lines.append('CC='+str(compiler).replace('\\','/'))
 assert shutil.which('openssl'),'openssl required in runner PATH (Git for Windows supplies it)'
else:lines.append('CC='+('clang' if platform.system()=='Darwin' else 'gcc'))
with Path(a.output).open('a') as out:out.write('\n'.join(lines)+'\n')
print('Configured native compiler for',platform.system(),platform.machine())
