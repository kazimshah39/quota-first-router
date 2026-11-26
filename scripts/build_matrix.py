"""Pinned cross toolchains; build all six candidates on Linux amd64.
macOS SDK download is explicit (--allow-sdk-download); alternatively supply
MACOS_SDK_PATH to an SDK you are entitled to use. Never publishes anything.
"""
import argparse
import hashlib
import importlib.util
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tarfile
import urllib.request

ROOT=Path(__file__).resolve().parents[1]
TOOLS=(
 ('zig-x86_64-linux-0.15.2','https://ziglang.org/download/0.15.2/zig-x86_64-linux-0.15.2.tar.xz','02aa270f183da276e5b5920b1dac44a63f1a49e55050ebde3aecc9eb82f93239'),
 ('llvm-mingw-20261117-ucrt-ubuntu-22.04-x86_64','https://github.com/mstorsjo/llvm-mingw/releases/download/20261117/llvm-mingw-20261117-ucrt-ubuntu-22.04-x86_64.tar.xz','e7a15a49a2ea95e9f3d8a9d0cfabe5052a4b10fb4777cc54de5a8c27b64bb701'))
SDK=('MacOSX26.1.sdk','https://github.com/joseluisq/macosx-sdks/releases/download/26.1/MacOSX26.1.sdk.tar.xz','beee7212d265a6d2867d0236cc069314b38d5fb3486a6515734e76fa210c784c')
def install(item,directory):
 name,url,digest=item;archive=directory/(name+'.tar.xz');marker=directory/(name+'.verified')
 if marker.exists() and marker.read_text()==digest and (directory/name).is_dir():return directory/name
 if not archive.exists():
  with urllib.request.urlopen(url,timeout=120) as response,archive.open('wb') as f:shutil.copyfileobj(response,f)
 assert hashlib.sha256(archive.read_bytes()).hexdigest()==digest,'Toolchain checksum mismatch'
 if (directory/name).exists():
  # SDK archives contain read-only files; remove only this tool-owned verified
  # download before re-extracting, never an operator-supplied SDK path.
  for path in (directory/name).rglob('*'):
   if not path.is_symlink():path.chmod(path.stat().st_mode | 0o200)
  shutil.rmtree(directory/name)
 with tarfile.open(archive) as tar:tar.extractall(directory,filter='data')
 assert (directory/name).is_dir();marker.write_text(digest);return directory/name

def main():
 p=argparse.ArgumentParser();p.add_argument('--allow-sdk-download',action='store_true');p.add_argument('--tools',default=str(ROOT/'.local/toolchains'));p.add_argument('--go',default=shutil.which('go') or str(Path.home()/'.local/share/go/bin/go'));p.add_argument('--output',default=str(ROOT/'dist/release'));a=p.parse_args();assert platform.system()=='Linux' and platform.machine() in ('x86_64','AMD64'),'Cross build coordinator needs Linux amd64'
 assert subprocess.check_output([a.go,'env','GOVERSION'],text=True).strip()=='go1.26.10','Candidate coordinator requires Go 1.26.10'
 tools=Path(a.tools).resolve();tools.mkdir(parents=True,exist_ok=True);zig=install(TOOLS[0],tools)/'zig';llvm=install(TOOLS[1],tools)/'bin'
 sdk=os.environ.get('MACOS_SDK_PATH')
 if not sdk:assert a.allow_sdk_download,'Supply MACOS_SDK_PATH or opt in to SDK download';sdk=str(install(SDK,tools))
 # CC can contain spaces, but paths containing spaces require a compiler wrapper.
 assert not any(' ' in str(x) for x in (zig,llvm,sdk)), 'Use toolchain paths without spaces'
 spec=importlib.util.spec_from_file_location('release',ROOT/'scripts/release.py');r=importlib.util.module_from_spec(spec);spec.loader.exec_module(r)
 for goos,arch in r.TARGETS:
  env=dict(os.environ);env.pop('CGO_LDFLAGS',None)
  if goos=='windows':env['CC']=str(llvm/('x86_64-w64-mingw32-clang' if arch=='amd64' else 'aarch64-w64-mingw32-clang'))
  elif goos=='darwin':
   env['CC']=f'{zig} cc -target '+('x86_64-macos' if arch=='amd64' else 'aarch64-macos')+f' -isysroot {sdk}';env['CGO_LDFLAGS']=f'-isysroot {sdk} -L{sdk}/usr/lib -F{sdk}/System/Library/Frameworks'
  else:env['CC']=f'{zig} cc -target '+('x86_64-linux-gnu.2.34' if arch=='amd64' else 'aarch64-linux-gnu.2.34')
  lib=ROOT/'dist'/f'{goos}-{arch}'/(r.ID+r.extension(goos))
  go=a.go
  if (goos,arch)==('darwin','amd64'):
   from darwin_toolchain import toolchain
   go=str(toolchain(tools,a.go));env['GOTOOLCHAIN']='local'
  subprocess.run(['make','package',f'GO={go}',f'TARGET_OS={goos}',f'TARGET_ARCH={arch}',f'LIBRARY={lib}',f'RELEASE_DIR={a.output}','BUILD_FLAGS=-O2'],cwd=ROOT,env=env,check=True)
  if (goos,arch)==('darwin','amd64'):
   disassembly=subprocess.check_output([str(llvm/'llvm-objdump'),'-d',str(lib)],text=True)
   assert '%gs:0x58' in disassembly and '%gs:0x30' not in disassembly,'Darwin TLS isolation not present'
 r.matrix(a.output);print('PASS six-platform build and binary/package matrix; native acceptance is a separate gate')
if __name__=='__main__':main()
