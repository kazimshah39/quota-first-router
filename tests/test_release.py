"""Offline packaging, native-header and six-artifact completeness regressions."""
import hashlib
import importlib.util
import json
from pathlib import Path
import struct
import tempfile
import unittest
from unittest.mock import patch
import zipfile

spec=importlib.util.spec_from_file_location('release',Path(__file__).resolve().parents[1]/'scripts/release.py');release=importlib.util.module_from_spec(spec);spec.loader.exec_module(release)

def fixture(goos,arch):
    raw=bytearray(1024);name=b'\0cliproxy_plugin_init\0'
    if goos=='linux':
        raw[:6]=b'\x7fELF\x02\x01';struct.pack_into('<HH',raw,16,3,62 if arch=='amd64' else 183);struct.pack_into('<Q',raw,40,64);struct.pack_into('<HH',raw,58,64,3)
        struct.pack_into('<IIQQQQIIQQ',raw,128,0,11,0,0,512,24,2,0,8,24);struct.pack_into('<IIQQQQIIQQ',raw,192,0,3,0,0,600,len(name),0,0,1,0);struct.pack_into('<IBBHQQ',raw,512,1,0x12,0,1,0,0);raw[600:600+len(name)]=name
    elif goos=='darwin':
        struct.pack_into('<IIIIIIII',raw,0,0xfeedfacf,0x1000007 if arch=='amd64' else 0x100000c,0,6,1,24,0,0);struct.pack_into('<IIIIII',raw,32,2,24,512,1,600,len(name));struct.pack_into('<IBBHQ',raw,512,1,0xf,1,0,0);raw[600:600+len(name)]=name
    else:
        raw[:2]=b'MZ';struct.pack_into('<I',raw,60,128);raw[128:132]=b'PE\0\0';struct.pack_into('<HHIIIHH',raw,132,0x8664 if arch=='amd64' else 0xaa64,1,0,0,0,240,0x2000);struct.pack_into('<H',raw,152,0x20b);struct.pack_into('<II',raw,264,0x1000,0);struct.pack_into('<8sIIIIIIHHI',raw,392,b'.rdata',512,0x1000,512,512,0,0,0,0,0);struct.pack_into('<I',raw,536,1);struct.pack_into('<I',raw,544,0x1040);struct.pack_into('<I',raw,576,0x1050);raw[592:592+len(name)-1]=name[1:]
    return bytes(raw)

class ReleaseTests(unittest.TestCase):
    def test_version_registry(self):
        self.assertRegex(release.version(),r'^\d+\.\d+\.\d+$');r=json.loads((release.ROOT/'registry.json').read_text());self.assertEqual(r['schema_version'],1);self.assertEqual(len(r['plugins']),1);self.assertEqual(r['plugins'][0]['id'],release.ID);self.assertEqual(r['plugins'][0]['install']['type'],'github-release')
    def test_all_binary_formats_machines_exports(self):
        for os,arch in release.TARGETS:
            with self.subTest(os=os,arch=arch):
                raw=fixture(os,arch);self.assertEqual(release.binary.inspect(raw,os,arch)['entry'],'cliproxy_plugin_init')
                with self.assertRaises(AssertionError):release.binary.inspect(raw,os,'arm64' if arch=='amd64' else 'amd64')
                with self.assertRaises(AssertionError):release.binary.inspect(raw.replace(b'cliproxy_plugin_init',b'no_plugin_entry____'),os,arch)
                with self.assertRaises((AssertionError,struct.error)):release.binary.inspect(raw[:32],os,arch)
    def test_runtime_baseline_rejection(self):
        raw=fixture('linux','amd64')+b'GLIBC_2.40\0'
        with self.assertRaisesRegex(AssertionError,'baseline'):release.binary.inspect(raw,'linux','amd64')

    def test_matrix_and_rejections(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);(root/'config.go').write_text('const pluginVersion = "0.1.2"');(root/'LICENSE').write_text('license');(root/'THIRD_PARTY_NOTICES.md').write_text('notices');out=root/'release'
            with patch.object(release,'ROOT',root):
                for os,arch in release.TARGETS:
                    lib=root/(release.ID+release.extension(os));lib.write_bytes(fixture(os,arch));release.package(os,arch,lib,out)
                self.assertEqual(len(release.matrix(out)),6)
                with self.assertRaises(FileNotFoundError):release.matrix(out,True)
                for os,arch in release.TARGETS:
                    (out/f'native-{os}-{arch}.json').write_text(json.dumps({'native':True,'goos':os,'goarch':arch,'passed':True,'library_sha256':hashlib.sha256(fixture(os,arch)).hexdigest(),'cpa_version':'8.1.0','local_store_install':True,'archive_sha256':hashlib.sha256((out/release.archive_name(os,arch)).read_bytes()).hexdigest(),'soak_seconds':75,'invalid_library_rejected':True,'cpa_sha256':'1'*64,'host_machine':arch,'go_version':'go1.26.10'}))
                self.assertEqual(len(release.matrix(out,True)),6)
                evidence=out/'native-windows-arm64.json';valid=evidence.read_text();bad=json.loads(valid);bad['library_sha256']='0'*64;evidence.write_text(json.dumps(bad))
                with self.assertRaises(AssertionError):release.matrix(out,True)
                evidence.write_text(valid)
                checks=out/'checksums.txt';original=checks.read_text();checks.write_text(original+original.splitlines()[0]+'\n')
                with self.assertRaises(AssertionError):release.matrix(out)
                checks.write_text(''.join(reversed(original.splitlines(keepends=True))))
                with self.assertRaises(AssertionError):release.matrix(out)
                checks.write_text(original.replace(original[:64],'0'*64))
                with self.assertRaises(AssertionError):release.matrix(out)
                checks.write_text(original);archive=out/release.archive_name('windows','arm64');backup=archive.read_bytes();archive.unlink()
                with self.assertRaises(AssertionError):release.matrix(out)
                archive.write_bytes(backup)
                with zipfile.ZipFile(archive,'a') as z:z.writestr('../unexpected.dll',b'bad')
                with self.assertRaises(AssertionError):release.validate_package(archive)
                archive.write_bytes(backup)
                with zipfile.ZipFile(archive,'a') as z:z.writestr(release.ID+'.dll',b'duplicate')
                with self.assertRaises(AssertionError):release.validate_package(archive)

if __name__=='__main__':unittest.main()
