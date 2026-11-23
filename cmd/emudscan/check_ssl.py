"""Test the ssl module of a Python before and after its files change.

eMudScan runs this script in a Python whose ssl files it replaces
(_ssl.pyd, the OpenSSL DLLs and, for some Pythons, _hashlib.pyd):

    python -I check_ssl.py FOLDER before
    python -I check_ssl.py FOLDER after

FOLDER holds the test certificates that eMudScan creates for each fix.
"before" saves the results in before.json, in FOLDER, and fails only
when the ssl and hash modules do not import: the fix then stops before
it changes anything. "after" fails when a test that passed before now
fails, or when the compromised test chain is still accepted: the fix
then puts the original files back.

The tests use the standard library only, and no network: the TLS
connections run in memory. -I (isolated mode) ignores the PYTHON*
environment variables and the user's site-packages, so that only the
files of this Python are tested. Each test imports its modules itself:
a broken module fails its own tests, and the other tests still run.

Tests:
    modules       -- the ssl and hash modules import
    hashes        -- hashes give their known results
    windows_roots -- the root certificates of Windows load
    control       -- TLS 1.2 and TLS 1.3 connections succeed
    denied        -- a chain through a compromised CA is rejected
"""

import json
import os
import sys

FOLDER, MODE = sys.argv[1], sys.argv[2]

# Results by test name: [passed, value or error message]. "before"
# saves them; "after" compares its own with them.
results = {}


def check(name, fn):
    """Run the test fn and save its result under name.

    Catch every exception, so that a failing test does not stop the
    others, and save its type and message as the result.
    """
    try:
        results[name] = [True, str(fn())]
    except BaseException as e:
        results[name] = [False, "%s: %s" % (type(e).__name__, e)]


def modules():
    """Import the ssl and hash modules.

    A replaced file that does not fit this Python fails here.
    Return the versions of Python and OpenSSL.
    """
    # The imports are the test: _ssl and _hashlib are the .pyd files
    # that the fix replaces.
    import ssl
    import _ssl
    import _hashlib
    import hashlib
    import hmac
    return "Python %s, %s" % (sys.version.split()[0], ssl.OPENSSL_VERSION)


def hashes():
    """Compare hashes with their known results (test vectors).

    OpenSSL computes most of them: a wrong result means that the
    OpenSSL DLLs do not work.
    Raise ValueError with the wrong ones.
    """
    import hashlib
    import hmac

    want = {
        "sha256":
            "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
        "sha512":  # its first 32 digits
            "ddaf35a193617abacc417349ae204131",
        "sha3_256":
            "3a985da74fe225b2045c172d6bd390bd855f086e3e9d525b46bfe24511431532",
        "md5":
            "900150983cd24fb0d6963f7d28e17f72",
        "hmac":
            "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8",
        "pbkdf2":
            "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b",
        "openssl_sha256":
            "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
    }

    fox = b"The quick brown fox jumps over the lazy dog"
    got = {
        "sha256": hashlib.sha256(b"abc").hexdigest(),
        "sha512": hashlib.sha512(b"abc").hexdigest()[:32],
        "sha3_256": hashlib.sha3_256(b"abc").hexdigest(),
        # usedforsecurity=False: MD5 works even where OpenSSL blocks it
        # for security uses (FIPS mode).
        "md5": hashlib.new("md5", b"abc", usedforsecurity=False).hexdigest(),
        "hmac": hmac.new(b"key", fox, "sha256").hexdigest(),
        "pbkdf2": hashlib.pbkdf2_hmac("sha256", b"password", b"salt", 1).hex(),
        "openssl_sha256": hashlib.new("sha256", b"abc").hexdigest(),
    }

    bad = [k for k in want if got[k] != want[k]]
    if bad:
        raise ValueError("wrong results: " + ", ".join(bad))
    return "%d known results" % len(want)


def windows_roots():
    """Count the root certificates that Python loads from Windows.

    Python checks HTTPS servers with these roots: without them, no
    HTTPS connection works.
    Raise ValueError if there are none.
    """
    import ssl
    n = ssl.create_default_context().cert_store_stats()["x509_ca"]
    if n == 0:
        raise ValueError("no root certificate loaded from Windows")
    return "%d roots" % n


def handshake(chain, ca, version):
    """Connect a TLS client to a TLS server, in memory.

    The server presents the certificate chain of the file chain. The
    client trusts only the CA of the file ca. Both use only version.
    Return the TLS version used.
    Raise ssl.SSLCertVerificationError if the client rejects the chain.
    """
    import ssl

    client_ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
    with open(os.path.join(FOLDER, ca)) as f:
        client_ctx.load_verify_locations(cadata=f.read())
    server_ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    server_ctx.load_cert_chain(os.path.join(FOLDER, chain))
    for ctx in (client_ctx, server_ctx):
        ctx.minimum_version = ctx.maximum_version = version

    # Memory buffers take the place of the network: the test needs no
    # socket. The test certificates are issued for emudscan.test.
    client_in, client_out = ssl.MemoryBIO(), ssl.MemoryBIO()
    server_in, server_out = ssl.MemoryBIO(), ssl.MemoryBIO()
    client = client_ctx.wrap_bio(client_in, client_out,
                                 server_hostname="emudscan.test")
    server = server_ctx.wrap_bio(server_in, server_out, server_side=True)

    # Each side goes on until it needs the other's data; then pass the
    # data across. A handshake takes 2 or 3 rounds: 20 is a safe limit.
    for _ in range(20):
        done = 0
        for side in (client, server):
            try:
                side.do_handshake()
                done += 1
            except ssl.SSLWantReadError:
                pass
        server_in.write(client_out.read())
        client_in.write(server_out.read())
        if done == 2:
            break

    client.write(b"ping")
    server_in.write(client_out.read())
    if server.read(4) != b"ping":
        raise ValueError("data not received")
    return client.version()


def control(version):
    """Return a test that connects with a valid test certificate.

    The connection must succeed: it shows that TLS works in this
    Python. check() runs the test.
    """
    return lambda: handshake("control.pem", "control-ca.pem", version)


def denied():
    """Check that a chain through a compromised CA is rejected.

    The test certificate is signed with the private key of the
    compromised intermediate "EM OV TLS CA - G3C", which anyone can
    compute (it is 4). A fixed Python blocks that intermediate.
    """
    import ssl
    try:
        handshake("denied.pem", "denied-ca.pem", ssl.TLSVersion.TLSv1_3)
    except ssl.SSLCertVerificationError as e:
        # 28 is X509_V_ERR_CERT_REJECTED, the error of a blocked
        # certificate. Any other error does not show the block.
        if e.verify_code != 28:
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
    # Without a working ssl module, the TLS tests cannot run: they fail
    # with its error. A test that already ran keeps its result.
    for n in ("TLS 1.2", "TLS 1.3", "block list"):
        results.setdefault(n, [False, "%s: %s" % (type(e).__name__, e)])

for name, (ok, v) in results.items():
    if ok:
        status = "OK"
    elif MODE == "before" and name == "block list":
        # Before the replacement, this Python is not fixed yet: it is
        # expected to accept the compromised chain.
        status = "not yet"
    else:
        status = "FAILED"
    print("%-7s %s: %s" % (status, name, v))

before = os.path.join(FOLDER, "before.json")
if MODE == "before":
    with open(before, "w") as f:
        json.dump(results, f)
    # Without its ssl and hash modules, this Python cannot be compared
    # after the replacement: fail, and the fix stops here.
    sys.exit(0 if results["modules"][0] else 1)

with open(before) as f:
    prev = json.load(f)
worse = [n for n, (ok, _) in prev.items()
         if ok and not results.get(n, [False])[0]]
if worse:
    print("Worse than before: " + ", ".join(worse))
# Fail when the new files broke a test, or do not block the compromised
# chain: the fix then puts the original files back.
sys.exit(1 if worse or not results["block list"][0] else 0)
