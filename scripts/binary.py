"""Inspect native plugin headers/exports/dependencies without loading untrusted code."""
import struct

ENTRY = 'cliproxy_plugin_init'

def inspect(data, goos, goarch):
    """Require a 64-bit shared library of the requested architecture and ABI export."""
    assert goos in ('linux', 'darwin', 'windows') and goarch in ('amd64', 'arm64'), 'Unsupported target'
    def unpack(fmt, offset):
        assert offset >= 0 and offset + struct.calcsize(fmt) <= len(data), 'Truncated binary'
        return struct.unpack_from(fmt, data, offset)
    def string(offset):
        assert 0 <= offset < len(data), 'Invalid string offset'
        end = data.find(b'\0', offset)
        assert end >= offset, 'Unterminated string'
        return data[offset:end].decode('utf-8', errors='strict')
    exports, dependencies, minimum = [], [], None
    if goos == 'linux':
        assert data[:6] == b'\x7fELF\x02\x01', 'Expected little-endian ELF64'
        assert unpack('<H', 16)[0] == 3, 'Expected shared ELF'
        assert unpack('<H', 18)[0] == {'amd64':62, 'arm64':183}[goarch], 'Wrong ELF machine'
        shoff = unpack('<Q',40)[0]; shsize, count = unpack('<HH',58)
        assert shsize >= 64 and count < 65536, 'Invalid sections'
        sections = [unpack('<IIQQQQIIQQ',shoff+i*shsize) for i in range(count)]
        for s in sections:
            if s[1] == 11:  # SHT_DYNSYM
                assert s[6] < len(sections) and s[9] >= 24, 'Invalid dynsym'
                strings = sections[s[6]][4]
                for i in range(s[5]//s[9]):
                    name, info, other, index, value, size = unpack('<IBBHQQ',s[4]+i*s[9])
                    if index and info >> 4 in (1,2) and other & 3 == 0:
                        exports.append(string(strings+name))
            if s[1] == 6:  # SHT_DYNAMIC
                assert s[6] < len(sections), 'Invalid dynamic strings'
                for i in range(s[5]//16):
                    tag, val = unpack('<qQ',s[4]+i*16)
                    if tag == 1: dependencies.append(string(sections[s[6]][4]+val))
        import re
        versions = sorted({m.decode() for m in re.findall(rb'GLIBC_[0-9]+\.[0-9]+',data)}, key=lambda v:tuple(map(int,v[6:].split('.'))))
        minimum = versions[-1] if versions else 'No GLIBC version references'
    elif goos == 'darwin':
        assert unpack('<I',0)[0] == 0xfeedfacf, 'Expected little-endian Mach-O64'
        cpu, subtype, kind, count = unpack('<IIII',4)
        assert cpu == {'amd64':0x1000007,'arm64':0x100000c}[goarch], 'Wrong Mach-O CPU'
        assert kind == 6 and count < 65536, 'Expected dylib'
        offset = 32
        for i in range(count):
            cmd, size = unpack('<II',offset); assert size >= 8, 'Invalid Mach-O command'
            if cmd == 2:  # LC_SYMTAB
                symoff,nsyms,stroff,strsize = unpack('<IIII',offset+8)
                for j in range(nsyms):
                    name, typ, sect, desc, value = unpack('<IBBHQ',symoff+j*16)
                    if typ & 1 and typ & 0x0e and not typ & 0xe0:
                        exports.append(string(stroff+name).removeprefix('_'))
            if cmd in (0xc,0x80000018,0x8000001f):
                dependencies.append(string(offset+unpack('<I',offset+8)[0]))
            if cmd == 0x32: minimum = unpack('<I',offset+12)[0]
            if cmd == 0x24: minimum = unpack('<I',offset+8)[0]
            offset += size
        if isinstance(minimum,int): minimum = f'{minimum>>16}.{minimum>>8&255}.{minimum&255}'
    else:
        assert data[:2] == b'MZ', 'Expected PE DLL'
        pe = unpack('<I',60)[0]; assert data[pe:pe+4] == b'PE\0\0', 'Invalid PE signature'
        machine,nsects,stamp,symoff,nsyms,optsize,flags = unpack('<HHIIIHH',pe+4)
        assert machine == {'amd64':0x8664,'arm64':0xaa64}[goarch], 'Wrong PE machine'
        assert flags & 0x2000, 'Expected DLL'
        opt=pe+24; assert unpack('<H',opt)[0] == 0x20b and optsize >= 128, 'Expected PE32+'
        sections=[unpack('<8sIIIIIIHHI',opt+optsize+i*40) for i in range(nsects)]
        def rva(value):
            for s in sections:
                if s[2] <= value < s[2]+s[3]: return s[4]+value-s[2]
            raise AssertionError('RVA outside file sections')
        export_rva=unpack('<I',opt+112)[0]
        assert export_rva, 'Missing PE export table'
        ex=rva(export_rva); n=unpack('<I',ex+24)[0]; names=rva(unpack('<I',ex+32)[0])
        for i in range(n): exports.append(string(rva(unpack('<I',names+4*i)[0])))
        import_rva=unpack('<I',opt+120)[0]
        if import_rva:
            pos=rva(import_rva)
            while True:
                row=unpack('<IIIII',pos)
                if row == (0,0,0,0,0): break
                dependencies.append(string(rva(row[3])));pos+=20
        minimum = 'PE subsystem %s.%s (not the Go runtime minimum)' % unpack('<HH',opt+48)
    assert ENTRY in exports, 'Missing public ABI entry point'
    if goos=='linux':
        assert set(dependencies)<= {'libc.so.6','libpthread.so.0','libdl.so.2','libresolv.so.2'},'Unexpected Linux runtime dependency'
        if minimum.startswith('GLIBC_'):assert tuple(map(int,minimum[6:].split('.'))) <= (2,34),'Linux glibc baseline exceeded'
    elif goos=='darwin':
        allowed={'/usr/lib/libSystem.B.dylib','/usr/lib/libobjc.A.dylib','/usr/lib/libresolv.9.dylib','/System/Library/Frameworks/CoreFoundation.framework/Versions/A/CoreFoundation','/System/Library/Frameworks/Security.framework/Versions/A/Security'}
        assert set(dependencies)<=allowed,'Unexpected Darwin runtime dependency'
        if minimum:assert tuple(map(int,minimum.split('.'))) <= (13,0,0),'Darwin deployment baseline exceeded'
    else:
        assert all(d.lower()=='kernel32.dll' or d.lower().startswith('api-ms-win-crt-') and d.lower().endswith('.dll') for d in dependencies),'Unexpected Windows runtime dependency'
    return {'goos':goos,'goarch':goarch,'entry':ENTRY,'dependencies':sorted(set(dependencies)), 'binary_minimum':minimum}
