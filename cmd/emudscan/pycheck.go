package main

// The checks of a Python's ssl module, which the replacement of its ssl files runs
// before and after (pysslfix.go): a Python script, with test certificates that eMudScan
// makes when the fix runs, valid one hour, written next to the fix's script. Standard
// library only, no network: the TLS connections run in memory.
//
//   - modules: ssl, _ssl, _hashlib, hashlib and hmac load (the new files load together),
//     and the version of OpenSSL;
//   - hashes: 7 known results (hashlib, hmac, pbkdf2, and OpenSSL's own SHA-256);
//   - Windows roots: the default context loads the roots of Windows;
//   - TLS 1.2 and TLS 1.3: a connection with a test certificate, data sent and received;
//   - block list: a chain through a compromised intermediate is rejected
//     (X509_V_ERR_CERT_REJECTED). Its leaf is signed with that intermediate's private
//     key, which is known: "EM OV TLS CA - G3C" has the private key 4, as in CPython's
//     own test.
//
// After the replacement, every check that passed before must pass, and the block list
// must reject the chain; otherwise the fix puts the original files back.

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	_ "embed"
	"encoding/pem"
	"errors"
	"math/big"
	"time"
)

// pyCheckScript is check_ssl.py, a file of the source code, included as it is:
// check_ssl.py <folder> before|after. Before, it writes its results into
// <folder>\before.json, and fails only when the ssl module does not load; after, it
// fails when a check that passed before fails, or when the block list does not reject
// the chain. It writes ASCII only.
//
//go:embed check_ssl.py
var pyCheckScript string

// pyCheckFiles are the files of the checks, for a fix that runs at now: the script, a
// throwaway CA's certificate for emudscan.test with its key (control.pem) and that CA
// (control-ca.pem), and a certificate for emudscan.test signed with the private key of
// the compromised intermediate "EM OV TLS CA - G3C", with its key and that
// intermediate (denied.pem), and its root, AAA Certificate Services (denied-ca.pem).
func pyCheckFiles(now time.Time) (map[string][]byte, error) {
	var inter *badCert
	for _, b := range badCerts {
		if b.Name == "EM OV TLS CA - G3C" && b.CrossSigned() {
			inter = b
		}
	}
	var root *rootCert
	for _, r := range rootCerts {
		if r.Name == rootNameAAA {
			root = r
		}
	}
	if inter == nil || root == nil {
		return nil, errors.New("reference data not loaded")
	}
	weak := weakIntermediateKey()
	if pub, ok := inter.Cert.PublicKey.(*ecdsa.PublicKey); !ok || !weak.PublicKey.Equal(pub) {
		return nil, errors.New("the key of EM OV TLS CA - G3C is not the expected one")
	}
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	ca := &x509.Certificate{SerialNumber: randomSerial(), Subject: pkix.Name{CommonName: "eMudScan test CA"},
		NotBefore: now.Add(-10 * time.Minute), NotAfter: now.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, MaxPathLenZero: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	if ca, err = x509.ParseCertificate(caDER); err != nil {
		return nil, err
	}
	control, err := testLeaf(now, ca, caKey)
	if err != nil {
		return nil, err
	}
	denied, err := testLeaf(now, inter.Cert, weak)
	if err != nil {
		return nil, err
	}
	certPEM := func(der []byte) []byte { return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}) }
	return map[string][]byte{
		pyCheckName:      []byte(pyCheckScript),
		"control.pem":    control,
		"control-ca.pem": certPEM(caDER),
		"denied.pem":     append(denied, certPEM(inter.DER)...),
		"denied-ca.pem":  []byte(root.PEM),
	}, nil
}

// weakIntermediateKey is the private key of "EM OV TLS CA - G3C": 4, on P-384.
func weakIntermediateKey() *ecdsa.PrivateKey {
	c := elliptic.P384()
	x, y := c.ScalarBaseMult(big.NewInt(4).FillBytes(make([]byte, 48)))
	return &ecdsa.PrivateKey{PublicKey: ecdsa.PublicKey{Curve: c, X: x, Y: y}, D: big.NewInt(4)}
}

// testLeaf is a certificate for emudscan.test, valid one hour from now, signed by
// parent's key, with its own key, both in PEM (key first).
func testLeaf(now time.Time, parent *x509.Certificate, parentKey crypto.Signer) ([]byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	leaf := &x509.Certificate{SerialNumber: randomSerial(), Subject: pkix.Name{CommonName: "emudscan.test"},
		NotBefore: now.Add(-10 * time.Minute), NotAfter: now.Add(time.Hour), DNSNames: []string{"emudscan.test"},
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, leaf, parent, &key.PublicKey, parentKey)
	if err != nil {
		return nil, err
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return append(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...), nil
}

// randomSerial is a random serial number of 127 bits.
func randomSerial() *big.Int {
	n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	return n.Add(n, big.NewInt(1))
}
