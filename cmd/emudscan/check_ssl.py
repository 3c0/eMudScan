# eMudScan's checks of a Python's ssl module, before and after it replaces its ssl
# files. Standard library only, no network: the TLS connections run in memory.
import sys, os, json
D, MODE = sys.argv[1], sys.argv[2]
res = {}
def check(name, fn):
    try:
        res[name] = [True, str(fn())]
    except BaseException as e:
        res[name] = [False, "%s: %s" % (type(e).__name__, e)]
def modules():
    import ssl, _ssl, _hashlib, hashlib, hmac
    return "Python %s, %s" % (sys.version.split()[0], ssl.OPENSSL_VERSION)
def hashes():
    import hashlib, hmac
    kat = {
        "sha256": (hashlib.sha256(b"abc").hexdigest(), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"),
        "sha512": (hashlib.sha512(b"abc").hexdigest()[:32], "ddaf35a193617abacc417349ae204131"),
        "sha3_256": (hashlib.sha3_256(b"abc").hexdigest(), "3a985da74fe225b2045c172d6bd390bd855f086e3e9d525b46bfe24511431532"),
        "md5": (hashlib.new("md5", b"abc", usedforsecurity=False).hexdigest(), "900150983cd24fb0d6963f7d28e17f72"),
        "hmac": (hmac.new(b"key", b"The quick brown fox jumps over the lazy dog", "sha256").hexdigest(), "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8"),
        "pbkdf2": (hashlib.pbkdf2_hmac("sha256", b"password", b"salt", 1).hex(), "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b"),
        "openssl_sha256": (hashlib.new("sha256", b"abc").hexdigest(), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"),
    }
    bad = [k for k, (got, want) in kat.items() if got != want]
    if bad:
        raise ValueError("wrong results: " + ", ".join(bad))
    return "%d known results" % len(kat)
def windows_roots():
    import ssl
    n = ssl.create_default_context().cert_store_stats()["x509_ca"]
    if n == 0:
        raise ValueError("no root certificate loaded from Windows")
    return "%d roots" % n
def handshake(chain, ca, version):
    import ssl
    client = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
    with open(os.path.join(D, ca)) as f:
        client.load_verify_locations(cadata=f.read())
    server = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    server.load_cert_chain(os.path.join(D, chain))
    for c in (client, server):
        c.minimum_version = c.maximum_version = version
    cin, cout, sin, sout = ssl.MemoryBIO(), ssl.MemoryBIO(), ssl.MemoryBIO(), ssl.MemoryBIO()
    c = client.wrap_bio(cin, cout, server_hostname="emudscan.test")
    s = server.wrap_bio(sin, sout, server_side=True)
    for _ in range(20):
        done = 0
        for obj in (c, s):
            try:
                obj.do_handshake()
                done += 1
            except ssl.SSLWantReadError:
                pass
        sin.write(cout.read())
        cin.write(sout.read())
        if done == 2:
            break
    c.write(b"ping")
    sin.write(cout.read())
    if s.read(4) != b"ping":
        raise ValueError("data not received")
    return c.version()
def control(version):
    return lambda: handshake("control.pem", "control-ca.pem", version)
def denied():
    import ssl
    try:
        handshake("denied.pem", "denied-ca.pem", ssl.TLSVersion.TLSv1_3)
    except ssl.SSLCertVerificationError as e:
        if e.verify_code != 28:  # X509_V_ERR_CERT_REJECTED: the block list
            raise
        return "rejects the compromised chain"
    raise ValueError("accepts the compromised chain")
check("modules", modules)
check("hashes", hashes)
check("Windows roots", windows_roots)
try:
    import ssl
    check("TLS 1.2", control(ssl.TLSVersion.TLSv1_2))
    check("TLS 1.3", control(ssl.TLSVersion.TLSv1_3))
    check("block list", denied)
except BaseException as e:
    for n in ("TLS 1.2", "TLS 1.3", "block list"):
        res.setdefault(n, [False, "%s: %s" % (type(e).__name__, e)])
for name, (ok, v) in res.items():
    print("%-7s %s: %s" % ("OK" if ok else ("not yet" if MODE == "before" and name == "block list" else "FAILED"), name, v))
before = os.path.join(D, "before.json")
if MODE == "before":
    with open(before, "w") as f:
        json.dump(res, f)
    sys.exit(0 if res["modules"][0] else 1)
with open(before) as f:
    prev = json.load(f)
worse = [n for n, (ok, _) in prev.items() if ok and not res.get(n, [False])[0]]
if worse:
    print("Worse than before: " + ", ".join(worse))
sys.exit(1 if worse or not res["block list"][0] else 0)
