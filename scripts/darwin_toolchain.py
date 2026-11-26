"""Build an isolated Go 1.26.10 toolchain for the Darwin amd64 plugin runtime.
CPA's own Go runtime uses TLS slot 6. A second stock Go runtime cannot share it.
Use reserved slot 11, following quota-reset-router's MIT-licensed workaround.
Never patches the user's Go installation. Native CPA acceptance remains required.
"""
import argparse
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import urllib.request

VERSION='1.26.10'
SOURCE_SHA256='fd63e883ba2f2bfca93c1646b31ebef63e2b1e4d2b317c9789d976a68d81f36f'

def toolchain(directory, bootstrap):
    directory=Path(directory).resolve();directory.mkdir(parents=True,exist_ok=True)
    target=directory/'go-darwin-tls';marker=directory/'go-darwin-tls.verified'
    if marker.exists() and marker.read_text()==SOURCE_SHA256 and (target/'bin/go').exists():return target/'bin/go'
    archive=directory/f'go{VERSION}.src.tar.gz'
    if not archive.exists():
        with urllib.request.urlopen(f'https://go.dev/dl/go{VERSION}.src.tar.gz',timeout=120) as response,archive.open('wb') as out:shutil.copyfileobj(response,out)
    assert hashlib.sha256(archive.read_bytes()).hexdigest()==SOURCE_SHA256,'Go source checksum mismatch'
    staging=directory/'_go-tls-source';shutil.rmtree(staging,ignore_errors=True);staging.mkdir()
    with tarfile.open(archive) as tar:tar.extractall(staging,filter='data')
    shutil.rmtree(target,ignore_errors=True);(staging/'go').rename(target);staging.rmdir()
    replacements={
        'src/cmd/link/internal/ld/sym.go':('ctxt.Tlsoffset = 0x30','ctxt.Tlsoffset = 0x58'),
        'src/runtime/sys_darwin_amd64.s':('MOVQ\tDX, 0x30(GS)','MOVQ\tDX, 0x58(GS)'),
    }
    for name,(old,new) in replacements.items():
        path=target/name;source=path.read_text();assert source.count(old)==1,name;path.write_text(source.replace(old,new))
    bootstrap_root=subprocess.check_output([bootstrap,'env','GOROOT'],text=True).strip()
    env=dict(os.environ,GOROOT_BOOTSTRAP=bootstrap_root,GOTOOLCHAIN='local',GOMAXPROCS='4')
    for key in ('GOROOT','GOOS','GOARCH','CC','CGO_CFLAGS','CGO_LDFLAGS'):env.pop(key,None)
    subprocess.run(['bash','make.bash'],cwd=target/'src',env=env,check=True)
    marker.write_text(SOURCE_SHA256);return target/'bin/go'

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('--tools',required=True);p.add_argument('--go',default='go');a=p.parse_args();print(toolchain(a.tools,a.go))
