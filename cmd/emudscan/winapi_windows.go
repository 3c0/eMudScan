package main

// Windows APIs used by the checks: registry, file version resources,
// CryptoAPI certificate stores and chain engine, WinHTTP.

import (
	"crypto/x509"
	"io/fs"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	modversion = sysDLL("version.dll")
	modwinhttp = sysDLL("winhttp.dll")
	modcrypt32 = sysDLL("crypt32.dll")
	modntdll   = sysDLL("ntdll.dll")
	modkernel  = kernel32

	procGetFileVersionInfoSizeW = modversion.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfoW     = modversion.NewProc("GetFileVersionInfoW")
	procVerQueryValueW          = modversion.NewProc("VerQueryValueW")

	procWinHttpOpen              = modwinhttp.NewProc("WinHttpOpen")
	procWinHttpConnect           = modwinhttp.NewProc("WinHttpConnect")
	procWinHttpOpenRequest       = modwinhttp.NewProc("WinHttpOpenRequest")
	procWinHttpSendRequest       = modwinhttp.NewProc("WinHttpSendRequest")
	procWinHttpReceiveResponse   = modwinhttp.NewProc("WinHttpReceiveResponse")
	procWinHttpReadData          = modwinhttp.NewProc("WinHttpReadData")
	procWinHttpQueryHeaders      = modwinhttp.NewProc("WinHttpQueryHeaders")
	procWinHttpCloseHandle       = modwinhttp.NewProc("WinHttpCloseHandle")
	procWinHttpSetTimeouts       = modwinhttp.NewProc("WinHttpSetTimeouts")
	procWinHttpSetOption         = modwinhttp.NewProc("WinHttpSetOption")
	procWinHttpSetStatusCallback = modwinhttp.NewProc("WinHttpSetStatusCallback")

	procCertGetEnhancedKeyUsage = modcrypt32.NewProc("CertGetEnhancedKeyUsage")

	procRtlGetVersion           = modntdll.NewProc("RtlGetVersion")
	procGetUserDefaultUILang    = modkernel.NewProc("GetUserDefaultUILanguage")
	procGetLogicalDrives        = modkernel.NewProc("GetLogicalDrives")
	procGetDriveTypeW           = modkernel.NewProc("GetDriveTypeW")
	procGetSystemWindowsDirectW = modkernel.NewProc("GetSystemWindowsDirectoryW")
)

// ---------------------------------------------------------------------------
// Registry

type regRoot = syscall.Handle

const (
	hkcu regRoot = syscall.HKEY_CURRENT_USER
	hklm regRoot = syscall.HKEY_LOCAL_MACHINE

	view64 uint32 = 0x0100 // KEY_WOW64_64KEY
	view32 uint32 = 0x0200 // KEY_WOW64_32KEY
)

func regOpen(root regRoot, path string, view uint32) (syscall.Handle, bool) {
	var h syscall.Handle
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, false
	}
	if syscall.RegOpenKeyEx(root, p, 0, syscall.KEY_READ|view, &h) != nil {
		return 0, false
	}
	return h, true
}

func regQuery(root regRoot, path, name string, view uint32) (uint32, []byte, bool) {
	h, ok := regOpen(root, path, view)
	if !ok {
		return 0, nil, false
	}
	defer syscall.RegCloseKey(h)
	var np *uint16
	if name != "" {
		np, _ = syscall.UTF16PtrFromString(name)
	}
	var typ, n uint32
	if syscall.RegQueryValueEx(h, np, nil, &typ, nil, &n) != nil {
		return 0, nil, false
	}
	buf := make([]byte, n+2)
	n = uint32(len(buf))
	if syscall.RegQueryValueEx(h, np, nil, &typ, &buf[0], &n) != nil {
		return 0, nil, false
	}
	return typ, buf[:n], true
}

func utf16BytesToString(b []byte) string {
	if len(b) < 2 {
		return ""
	}
	u := unsafe.Slice((*uint16)(unsafe.Pointer(&b[0])), len(b)/2)
	return syscall.UTF16ToString(u)
}

func regString(root regRoot, path, name string, view uint32) (string, bool) {
	typ, b, ok := regQuery(root, path, name, view)
	if !ok || (typ != syscall.REG_SZ && typ != syscall.REG_EXPAND_SZ) {
		return "", false
	}
	s := utf16BytesToString(b)
	if typ == syscall.REG_EXPAND_SZ {
		s = expandEnv(s)
	}
	return s, true
}

func regDword(root regRoot, path, name string, view uint32) (uint32, bool) {
	typ, b, ok := regQuery(root, path, name, view)
	if !ok || typ != syscall.REG_DWORD || len(b) < 4 {
		return 0, false
	}
	return *(*uint32)(unsafe.Pointer(&b[0])), true
}

func regBinary(root regRoot, path, name string, view uint32) ([]byte, bool) {
	typ, b, ok := regQuery(root, path, name, view)
	if !ok || typ != syscall.REG_BINARY {
		return nil, false
	}
	return b, true
}

func regSubkeys(root regRoot, path string, view uint32) []string {
	h, ok := regOpen(root, path, view)
	if !ok {
		return nil
	}
	defer syscall.RegCloseKey(h)
	var out []string
	for i := uint32(0); i < 100000; i++ {
		buf := make([]uint16, 512)
		n := uint32(len(buf))
		if syscall.RegEnumKeyEx(h, i, &buf[0], &n, nil, nil, nil, nil) != nil {
			break
		}
		out = append(out, syscall.UTF16ToString(buf[:n]))
	}
	return out
}

func expandEnv(s string) string {
	// %VAR% expansion without calling into kernel32
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '%')
		if i < 0 {
			break
		}
		j := strings.IndexByte(s[i+1:], '%')
		if j < 0 {
			break
		}
		name := s[i+1 : i+1+j]
		b.WriteString(s[:i])
		if v := getenv(name); v != "" {
			b.WriteString(v)
		} else {
			b.WriteString("%" + name + "%")
		}
		s = s[i+j+2:]
	}
	b.WriteString(s)
	return b.String()
}

// ---------------------------------------------------------------------------
// File version resources

type fileVersion struct {
	Fixed   Version // binary FILEVERSION
	Product string  // ProductVersion string
	File    string  // FileVersion string
	Name    string  // ProductName
	Desc    string  // FileDescription
	Company string  // CompanyName
}

type vsFixedFileInfo struct {
	Signature, StrucVersion, FileVersionMS, FileVersionLS, ProductVersionMS, ProductVersionLS uint32
	FileFlagsMask, FileFlags, FileOS, FileType, FileSubtype, FileDateMS, FileDateLS           uint32
}

func getFileVersion(path string) (fileVersion, bool) {
	var fv fileVersion
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return fv, false
	}
	size, _, _ := procGetFileVersionInfoSizeW.Call(uintptr(unsafe.Pointer(p)), 0)
	if size == 0 || size > 4<<20 {
		return fv, false
	}
	buf := make([]byte, size)
	if r, _, _ := procGetFileVersionInfoW.Call(uintptr(unsafe.Pointer(p)), 0, size, uintptr(unsafe.Pointer(&buf[0]))); r == 0 {
		return fv, false
	}
	query := func(sub string) (unsafe.Pointer, uint32) {
		sp, _ := syscall.UTF16PtrFromString(sub)
		var p unsafe.Pointer // points into buf
		var n uint32
		r, _, _ := procVerQueryValueW.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(sp)), uintptr(unsafe.Pointer(&p)), uintptr(unsafe.Pointer(&n)))
		if r == 0 || p == nil {
			return nil, 0
		}
		return p, n
	}
	if p, n := query(`\`); p != nil && n >= uint32(unsafe.Sizeof(vsFixedFileInfo{})) {
		fi := (*vsFixedFileInfo)(p)
		if fi.Signature == 0xFEEF04BD {
			fv.Fixed = Version{int(fi.FileVersionMS >> 16), int(fi.FileVersionMS & 0xffff), int(fi.FileVersionLS >> 16), int(fi.FileVersionLS & 0xffff)}
		}
	}
	var langs []string
	if p, n := query(`\VarFileInfo\Translation`); p != nil && n >= 4 {
		pairs := unsafe.Slice((*uint16)(p), n/2)
		for i := 0; i+1 < len(pairs); i += 2 {
			langs = append(langs, strings.ToLower(hex4(pairs[i])+hex4(pairs[i+1])))
		}
	}
	langs = append(langs, "040904b0", "040904e4", "000004b0")
	str := func(key string) string {
		for _, l := range langs {
			if p, n := query(`\StringFileInfo\` + l + `\` + key); p != nil && n > 0 {
				return strings.TrimSpace(syscall.UTF16ToString(unsafe.Slice((*uint16)(p), n)))
			}
		}
		return ""
	}
	fv.Product, fv.File, fv.Name, fv.Desc = str("ProductVersion"), str("FileVersion"), str("ProductName"), str("FileDescription")
	fv.Company = str("CompanyName")
	return fv, true
}

func hex4(v uint16) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[v>>12&15], digits[v>>8&15], digits[v>>4&15], digits[v&15]})
}

// ---------------------------------------------------------------------------
// Files and drives

const (
	fileAttrOffline            = 0x00001000
	fileAttrRecallOnOpen       = 0x00040000
	fileAttrRecallOnDataAccess = 0x00400000
)

// isCloudPlaceholder reports files whose content is not on disk (OneDrive "online-only").
// Reading them would trigger a download.
func isCloudPlaceholder(fi fs.FileInfo) bool {
	if d, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		return d.FileAttributes&(fileAttrOffline|fileAttrRecallOnOpen|fileAttrRecallOnDataAccess) != 0
	}
	return false
}

// creationTime is when the file was created on this PC.
func creationTime(fi fs.FileInfo) (time.Time, bool) {
	if d, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		return time.Unix(0, d.CreationTime.Nanoseconds()), true
	}
	return time.Time{}, false
}

var procGetDateFormatEx = modkernel.NewProc("GetDateFormatEx")

// shortDate formats a date as Windows does in Explorer (the user's short date format).
func shortDate(t time.Time) string {
	t = t.Local()
	st := [8]uint16{uint16(t.Year()), uint16(t.Month()), uint16(t.Weekday()), uint16(t.Day())}
	var buf [80]uint16
	n, _, _ := procGetDateFormatEx.Call(0, 1 /* DATE_SHORTDATE */, uintptr(unsafe.Pointer(&st[0])), 0,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	if n == 0 {
		return t.Format("2006-01-02")
	}
	return syscall.UTF16ToString(buf[:n])
}

// driveInfo describes a drive for the drive checkboxes.
type driveInfo struct {
	Root  string // "C:\"
	Label string // as shown by Explorer, e.g. "Local Disk (C:)"
	Fixed bool   // local hard drive or SSD: selected by default
}

var (
	modshell32w        = sysDLL("shell32.dll")
	procSHGetFileInfoW = modshell32w.NewProc("SHGetFileInfoW")
	procSetErrorMode   = modkernel.NewProc("SetErrorMode")
)

// listDrives returns the drives of the computer. Explorer's display name is used
// for local and network drives; optical and removable drives are not touched.
func listDrives() []driveInfo {
	procSetErrorMode.Call(0x0001 | 0x8000) // no "insert a disk" dialogs
	mask, _, _ := procGetLogicalDrives.Call()
	var out []driveInfo
	for i := 0; i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		p, _ := syscall.UTF16PtrFromString(root)
		t, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(p)))
		d := driveInfo{Root: root, Fixed: t == 3, Label: root[:2]}
		switch t {
		case 0, 1: // unknown, no root
			continue
		case 2:
			d.Label += " " + T("drive.removable")
		case 5:
			d.Label += " " + T("drive.optical")
		case 4:
			d.Label += " " + T("drive.network")
		case 6:
			d.Label += " " + T("drive.ram")
		}
		if t == 3 {
			var fi struct {
				Icon        uintptr
				IconIdx     int32
				Attributes  uint32
				DisplayName [260]uint16
				TypeName    [80]uint16
			}
			if r, _, _ := procSHGetFileInfoW.Call(uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(&fi)), unsafe.Sizeof(fi), 0x200 /* SHGFI_DISPLAYNAME */); r != 0 {
				if n := syscall.UTF16ToString(fi.DisplayName[:]); n != "" && n != "("+root[:2]+")" {
					d.Label = n
				}
			}
		}
		out = append(out, d)
	}
	return out
}

func fixedDrives() []string {
	mask, _, _ := procGetLogicalDrives.Call()
	var out []string
	for i := 0; i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		p, _ := syscall.UTF16PtrFromString(root)
		if t, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(p))); t == 3 { // DRIVE_FIXED
			out = append(out, root)
		}
	}
	return out
}

func windowsDir() string {
	buf := make([]uint16, 260)
	n, _, _ := procGetSystemWindowsDirectW.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 || int(n) > len(buf) {
		return `C:\Windows`
	}
	return syscall.UTF16ToString(buf[:n])
}

func userUILanguage() string {
	id, _, _ := procGetUserDefaultUILang.Call()
	if id&0x3ff == 0x0c { // LANG_FRENCH
		return "fr"
	}
	return "en"
}

type osVersionInfoEx struct {
	Size                          uint32
	Major, Minor, Build, Platform uint32
	CSDVersion                    [128]uint16
	SPMajor, SPMinor, SuiteMask   uint16
	ProductType, Reserved         byte
}

// windowsEditions: the marketing names of Windows editions (EditionID in the
// registry, whose ProductName still says "Windows 10" on Windows 11).
var windowsEditions = map[string]string{
	"Core": "Home", "CoreN": "Home N", "CoreSingleLanguage": "Home Single Language", "CoreCountrySpecific": "Home China",
	"Professional": "Pro", "ProfessionalN": "Pro N", "ProfessionalEducation": "Pro Education", "ProfessionalEducationN": "Pro Education N",
	"ProfessionalWorkstation": "Pro for Workstations", "ProfessionalWorkstationN": "Pro for Workstations N",
	"Enterprise": "Enterprise", "EnterpriseN": "Enterprise N", "EnterpriseS": "Enterprise LTSC", "EnterpriseSN": "Enterprise N LTSC",
	"EnterpriseG": "Enterprise G", "IoTEnterprise": "IoT Enterprise", "IoTEnterpriseS": "IoT Enterprise LTSC",
	"Education": "Education", "EducationN": "Education N", "ServerRdsh": "Enterprise multi-session", "CloudEdition": "SE",
}

// windowsVersion describes this Windows: its name with the edition and the yearly
// version ("Windows 11 Pro 25H2"), and its build ("26200.9711", with the major
// and minor version: "10.0.26200.9711").
func windowsVersion() (name, build, full string) {
	var vi osVersionInfoEx
	vi.Size = uint32(unsafe.Sizeof(vi))
	procRtlGetVersion.Call(uintptr(unsafe.Pointer(&vi)))
	const cv = `SOFTWARE\Microsoft\Windows NT\CurrentVersion`
	name = "Windows"
	if vi.Major == 10 && vi.Build >= 22000 {
		name = "Windows 11"
	} else if vi.Major == 10 {
		name = "Windows 10"
	}
	if vi.ProductType != 1 { // a server: its product name is right
		if pn, _ := regString(hklm, cv, "ProductName", view64); pn != "" {
			name = pn
		}
	} else if ed, _ := regString(hklm, cv, "EditionID", view64); ed != "" {
		if e, ok := windowsEditions[ed]; ok {
			ed = e
		}
		name += " " + ed
	}
	if disp, _ := regString(hklm, cv, "DisplayVersion", view64); disp != "" {
		name += " " + disp
	}
	ubr, _ := regDword(hklm, cv, "UBR", view64)
	build = itoa(int(vi.Build)) + "." + itoa(int(ubr))
	return name, build, itoa(int(vi.Major)) + "." + itoa(int(vi.Minor)) + "." + build
}

func windowsVersionString() string {
	name, _, full := windowsVersion()
	return name + " (" + full + ")"
}

// userAgent is sent with the requests of the live test, where the advisory's author
// sees it: the version of eMudScan, then (in the comment) its date, the version of
// Windows and where eMudScan comes from:
// "eMudScan/1.0 (20261026; Windows 11 Pro 25H2 26200.9711; +https://github.com/3c0/eMudScan)".
func userAgent() string {
	name, build, _ := windowsVersion()
	return appName + "/" + appVersion + " (" + appDate + "; " + name + " " + build + "; +" + appURL + ")"
}

// ---------------------------------------------------------------------------
// Certificates

const (
	certTrustIsRevoked          = 0x00000004
	certTrustIsExplicitDistrust = 0x04000000
	certChainCacheOnlyURL       = 0x00000004 // CERT_CHAIN_CACHE_ONLY_URL_RETRIEVAL: no network
	certChainNoRootDownload     = 0x00000100 // CERT_CHAIN_DISABLE_AUTH_ROOT_AUTO_UPDATE: no root download
	certStoreProvSystemW        = 10
	certSystemStoreLocalMachine = 2 << 16
	certSystemStoreCurrentUser  = 1 << 16
	certStoreReadOnly           = 0x00008000
	certStoreOpenExisting       = 0x00004000
	cryptENotFound              = 0x80092004
)

// chainErrorStatus builds a chain for the DER certificate with the Windows chain
// engine and returns the combined trust error status. No network access, and no
// automatic update of Windows' certificate lists: eMudScan changes nothing.
func chainErrorStatus(der []byte) (uint32, error) {
	ctx, err := syscall.CertCreateCertificateContext(syscall.X509_ASN_ENCODING|syscall.PKCS_7_ASN_ENCODING, &der[0], uint32(len(der)))
	if err != nil {
		return 0, err
	}
	defer syscall.CertFreeCertificateContext(ctx)
	para := syscall.CertChainPara{}
	para.Size = uint32(unsafe.Sizeof(para))
	var chain *syscall.CertChainContext
	if err := syscall.CertGetCertificateChain(0, ctx, nil, 0, &para, certChainCacheOnlyURL|certChainNoRootDownload, 0, &chain); err != nil {
		return 0, err
	}
	defer syscall.CertFreeCertificateChain(chain)
	return chain.TrustStatus.ErrorStatus, nil
}

// sslChainCheck submits a test chain (the leaf, then its intermediates) to the checks
// Windows applies to the certificates of a secure connection (WinHTTP, Schannel): the
// chain engine, then the policy of TLS servers for host. No network is used: certificate
// downloads and the automatic installation of roots are disabled. The chain is checked
// at a date when the leaf was valid, so that a test certificate that has expired since
// still says whether the chain is blocked (at). It returns the chain's trust error status
// and the policy's error (0: Windows would accept the connection).
func sslChainCheck(host string, leaf []byte, intermediates [][]byte, at time.Time) (trustErrors, policyErr uint32, err error) {
	const enc = syscall.X509_ASN_ENCODING | syscall.PKCS_7_ASN_ENCODING
	store, err := syscall.CertOpenStore(syscall.CERT_STORE_PROV_MEMORY, 0, 0, syscall.CERT_STORE_DEFER_CLOSE_UNTIL_LAST_FREE_FLAG, 0)
	if err != nil {
		return 0, 0, err
	}
	defer syscall.CertCloseStore(store, 0)
	leafCtx, err := syscall.CertCreateCertificateContext(enc, &leaf[0], uint32(len(leaf)))
	if err != nil {
		return 0, 0, err
	}
	var inStore *syscall.CertContext
	err = syscall.CertAddCertificateContextToStore(store, leafCtx, syscall.CERT_STORE_ADD_ALWAYS, &inStore)
	syscall.CertFreeCertificateContext(leafCtx)
	if err != nil {
		return 0, 0, err
	}
	defer syscall.CertFreeCertificateContext(inStore)
	for _, der := range intermediates {
		ctx, err := syscall.CertCreateCertificateContext(enc, &der[0], uint32(len(der)))
		if err != nil {
			return 0, 0, err
		}
		err = syscall.CertAddCertificateContextToStore(store, ctx, syscall.CERT_STORE_ADD_ALWAYS, nil)
		syscall.CertFreeCertificateContext(ctx)
		if err != nil {
			return 0, 0, err
		}
	}
	serverAuth := []*byte{&syscall.OID_PKIX_KP_SERVER_AUTH[0]}
	para := syscall.CertChainPara{}
	para.Size = uint32(unsafe.Sizeof(para))
	para.RequestedUsage.Type = syscall.USAGE_MATCH_TYPE_OR
	para.RequestedUsage.Usage.Length = 1
	para.RequestedUsage.Usage.UsageIdentifiers = &serverAuth[0]
	when := syscall.NsecToFiletime(at.UnixNano())
	var chain *syscall.CertChainContext
	if err := syscall.CertGetCertificateChain(0, inStore, &when, store, &para, certChainCacheOnlyURL|certChainNoRootDownload, 0, &chain); err != nil {
		return 0, 0, err
	}
	defer syscall.CertFreeCertificateChain(chain)
	name, _ := syscall.UTF16PtrFromString(host)
	ssl := syscall.SSLExtraCertChainPolicyPara{AuthType: syscall.AUTHTYPE_SERVER, ServerName: name}
	ssl.Size = uint32(unsafe.Sizeof(ssl))
	ppara := syscall.CertChainPolicyPara{ExtraPolicyPara: syscall.Pointer(unsafe.Pointer(&ssl))}
	ppara.Size = uint32(unsafe.Sizeof(ppara))
	status := syscall.CertChainPolicyStatus{}
	status.Size = uint32(unsafe.Sizeof(status))
	if err := syscall.CertVerifyCertificateChainPolicy(syscall.CERT_CHAIN_POLICY_SSL, chain, &ppara, &status); err != nil {
		return 0, 0, err
	}
	return chain.TrustStatus.ErrorStatus, status.Error, nil
}

func enumStore(store syscall.Handle, keep func(ctx *syscall.CertContext) bool) [][]byte {
	var out [][]byte
	var ctx *syscall.CertContext
	for {
		var err error
		ctx, err = syscall.CertEnumCertificatesInStore(store, ctx)
		if err != nil || ctx == nil {
			break
		}
		if keep != nil && !keep(ctx) {
			continue
		}
		b := unsafe.Slice(ctx.EncodedCert, ctx.Length)
		out = append(out, append([]byte(nil), b...))
	}
	return out
}

// systemStoreCerts returns the certificates of a logical system store of the
// current user ("ROOT", "CA", ...), which includes the machine's stores.
func systemStoreCerts(name string, keep func(ctx *syscall.CertContext) bool) [][]byte {
	p, _ := syscall.UTF16PtrFromString(name)
	store, err := syscall.CertOpenSystemStore(0, p)
	if err != nil {
		return nil
	}
	defer syscall.CertCloseStore(store, 0)
	return enumStore(store, keep)
}

// machineStoreCerts reads a LocalMachine system store (e.g. "Disallowed") read-only.
func machineStoreCerts(name string) [][]byte {
	p, _ := syscall.UTF16PtrFromString(name)
	store, err := syscall.CertOpenStore(certStoreProvSystemW, 0, 0,
		certSystemStoreLocalMachine|certStoreReadOnly|certStoreOpenExisting, uintptr(unsafe.Pointer(p)))
	if err != nil {
		return nil
	}
	defer syscall.CertCloseStore(store, 0)
	return enumStore(store, nil)
}

const oidServerAuth = "1.3.6.1.5.5.7.3.1"

// trustedForServerAuth mirrors what Python's ssl module does on Windows
// (enum_certificates): extended key usage properties first, then the extension.
func trustedForServerAuth(ctx *syscall.CertContext) bool {
	usages, all := certUsages(ctx, 0x4) // CERT_FIND_PROP_ONLY_ENHKEY_USAGE_FLAG
	if all {
		usages, all = certUsages(ctx, 0x2) // CERT_FIND_EXT_ONLY_ENHKEY_USAGE_FLAG
	}
	if all {
		return true
	}
	for _, u := range usages {
		if u == oidServerAuth {
			return true
		}
	}
	return false
}

func certUsages(ctx *syscall.CertContext, flags uint32) (usages []string, all bool) {
	var size uint32
	r, _, err := procCertGetEnhancedKeyUsage.Call(uintptr(unsafe.Pointer(ctx)), uintptr(flags), 0, uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		if errno, ok := err.(syscall.Errno); ok && uint32(errno) == cryptENotFound {
			return nil, true
		}
		return nil, false
	}
	buf := make([]byte, size+8)
	r, _, err = procCertGetEnhancedKeyUsage.Call(uintptr(unsafe.Pointer(ctx)), uintptr(flags), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return nil, false
	}
	type ctlUsage struct {
		Count uint32
		Ids   **byte
	}
	u := (*ctlUsage)(unsafe.Pointer(&buf[0]))
	if u.Count == 0 {
		if errno, ok := err.(syscall.Errno); ok && uint32(errno) == cryptENotFound {
			return nil, true
		}
		return nil, false
	}
	ids := unsafe.Slice(u.Ids, u.Count)
	for _, id := range ids {
		usages = append(usages, cString(id))
	}
	return usages, false
}

func cString(p *byte) string {
	if p == nil {
		return ""
	}
	var b []byte
	for i := 0; i < 256; i++ {
		c := *(*byte)(unsafe.Add(unsafe.Pointer(p), i))
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	return string(b)
}

func parseCerts(ders [][]byte) []*x509.Certificate {
	var out []*x509.Certificate
	for _, d := range ders {
		if c, err := x509.ParseCertificate(d); err == nil {
			out = append(out, c)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// WinHTTP

type httpsResult struct {
	Connected   bool
	Status      int
	Body        string
	Err         uint32 // Win32 error code
	SecureFlags uint32 // WINHTTP_CALLBACK_STATUS_FLAG_* from the secure failure callback
}

var (
	secureFailMu    sync.Mutex
	secureFailFlags = map[uintptr]uint32{}
	secureFailNext  uintptr
	winhttpCallback uintptr
	callbackOnce    sync.Once
)

func winhttpStatusCallback(h, context, status, info, infoLen uintptr) uintptr {
	if uint32(status) == 0x00010000 && info != 0 { // WINHTTP_CALLBACK_STATUS_SECURE_FAILURE
		flags := *(*uint32)(ptr(info))
		secureFailMu.Lock()
		secureFailFlags[context] |= flags
		secureFailMu.Unlock()
	}
	return 0
}

func winhttpGet(host string, timeoutMs int) httpsResult {
	var res httpsResult
	if err := procWinHttpOpen.Find(); err != nil {
		res.Err = 127
		return res
	}
	callbackOnce.Do(func() { winhttpCallback = syscall.NewCallback(winhttpStatusCallback) })
	ua, _ := syscall.UTF16PtrFromString(userAgent())
	sess, _, err := procWinHttpOpen.Call(uintptr(unsafe.Pointer(ua)), 4 /* AUTOMATIC_PROXY */, 0, 0, 0)
	if sess == 0 {
		sess, _, err = procWinHttpOpen.Call(uintptr(unsafe.Pointer(ua)), 0 /* DEFAULT_PROXY */, 0, 0, 0)
	}
	if sess == 0 {
		res.Err = errno(err)
		return res
	}
	defer procWinHttpCloseHandle.Call(sess)
	t := uintptr(timeoutMs)
	procWinHttpSetTimeouts.Call(sess, t, t, t, t)
	hp, _ := syscall.UTF16PtrFromString(host)
	conn, _, err := procWinHttpConnect.Call(sess, uintptr(unsafe.Pointer(hp)), 443, 0)
	if conn == 0 {
		res.Err = errno(err)
		return res
	}
	defer procWinHttpCloseHandle.Call(conn)
	verb, _ := syscall.UTF16PtrFromString("GET")
	obj, _ := syscall.UTF16PtrFromString("/?client=" + strings.ToLower(appName)) // "/?client=emudscan"
	req, _, err := procWinHttpOpenRequest.Call(conn, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(obj)), 0, 0, 0, 0x00800000 /* WINHTTP_FLAG_SECURE */)
	if req == 0 {
		res.Err = errno(err)
		return res
	}
	defer procWinHttpCloseHandle.Call(req)
	// The answer must come from host itself, not from a site it redirects to, and
	// nothing is sent beyond the request: no redirection is followed, no cookie is
	// kept or sent. (Windows' credentials are not sent either: WinHTTP's default
	// policy sends them only to the servers of the proxy bypass list, never to the
	// internet; it is left as is, so that the test works through company proxies.)
	disabled := uint32(0x1 | 0x2) // WINHTTP_DISABLE_COOKIES | WINHTTP_DISABLE_REDIRECTS
	procWinHttpSetOption.Call(req, 63 /* WINHTTP_OPTION_DISABLE_FEATURE */, uintptr(unsafe.Pointer(&disabled)), 4)
	secureFailMu.Lock()
	secureFailNext++
	ctxID := secureFailNext
	secureFailMu.Unlock()
	procWinHttpSetStatusCallback.Call(req, winhttpCallback, 0x00010000 /* FLAG_SECURE_FAILURE */, 0)
	ok, _, err := procWinHttpSendRequest.Call(req, 0, 0, 0, 0, 0, ctxID)
	if ok != 0 {
		ok, _, err = procWinHttpReceiveResponse.Call(req, 0)
	}
	secureFailMu.Lock()
	res.SecureFlags = secureFailFlags[ctxID]
	delete(secureFailFlags, ctxID)
	secureFailMu.Unlock()
	if ok == 0 {
		res.Err = errno(err)
		return res
	}
	res.Connected = true
	var code, size uint32 = 0, 4
	procWinHttpQueryHeaders.Call(req, 19|0x20000000 /* STATUS_CODE | FLAG_NUMBER */, 0, uintptr(unsafe.Pointer(&code)), uintptr(unsafe.Pointer(&size)), 0)
	res.Status = int(code)
	buf := make([]byte, 4096)
	total := 0
	for total < len(buf) {
		var n uint32
		r, _, _ := procWinHttpReadData.Call(req, uintptr(unsafe.Pointer(&buf[total])), uintptr(len(buf)-total), uintptr(unsafe.Pointer(&n)))
		if r == 0 || n == 0 {
			break
		}
		total += int(n)
	}
	res.Body = string(buf[:total])
	return res
}

func errno(err error) uint32 {
	if e, ok := err.(syscall.Errno); ok {
		return uint32(e)
	}
	return 0xFFFFFFFF
}
